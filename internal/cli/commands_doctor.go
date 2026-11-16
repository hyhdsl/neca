package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func newDoctorCommand() Command {
	return Command{
		Name:    "doctor",
		Summary: "诊断配置、PKI、日志、Web 与 helper 状态",
		Group:   "meta",
		Surface: SurfaceBackend,
		Run:     runDoctor,
	}
}

func runDoctor(env Env) int {
	cfg, err := core.LoadConfigReadOnly()
	if err != nil {
		return fail(env, err)
	}
	checks := []doctorCheck{{
		Name: "config", Status: "OK", Detail: cfg.ConfigFile,
	}}
	addFileCheck := func(name, path string) {
		info, err := os.Stat(path)
		if err != nil {
			checks = append(checks, doctorCheck{Name: name, Status: "FAIL", Detail: path + ": " + err.Error()})
			return
		}
		checks = append(checks, doctorCheck{Name: name, Status: "OK", Detail: fmt.Sprintf("%s (%s)", path, info.Mode().Perm())})
	}
	addFileCheck("caCert", cfg.CACert)
	addFileCheck("caKey", cfg.CAKey)
	addFileCheck("index", cfg.IndexFile)
	addFileCheck("crl", cfg.CRLFile)
	if err := cfg.Web.Validate(); err != nil {
		checks = append(checks, doctorCheck{Name: "webConfig", Status: "FAIL", Detail: err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "webConfig", Status: "OK", Detail: cfg.Web.Listen})
	}
	if _, err := exec.LookPath(cfg.DockerBin); err != nil {
		checks = append(checks, doctorCheck{Name: "docker", Status: "WARN", Detail: cfg.DockerBin + " not found: " + err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "docker", Status: "OK", Detail: cfg.DockerBin})
	}
	if _, err := os.Stat(cfg.AuditFile); err != nil && !os.IsNotExist(err) {
		checks = append(checks, doctorCheck{Name: "audit", Status: "FAIL", Detail: err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "audit", Status: "OK", Detail: cfg.AuditFile})
	}
	helperClient := helper.NewClient(helperSocketPath())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	status, helperErr := helperClient.Status(ctx)
	cancel()
	if helperErr != nil {
		checks = append(checks, doctorCheck{Name: "helper", Status: "WARN", Detail: helperErr.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "helper", Status: "OK", Detail: status.Version})
	}
	failed := false
	for _, check := range checks {
		if check.Status == "FAIL" {
			failed = true
		}
	}
	if env.JSON {
		code := ExitOK
		if failed {
			code = ExitRuntime
		}
		result := emitJSON(env, "doctor", "", nil, map[string]any{"checks": checks, "ok": !failed})
		if code == ExitRuntime {
			return ExitRuntime
		}
		return result
	}
	for _, check := range checks {
		prefix := "[OK]  "
		if check.Status == "WARN" {
			prefix = "[WARN]"
		} else if check.Status == "FAIL" {
			prefix = "[ERR] "
		}
		fmt.Fprintf(env.Stdout, "%s %-10s %s\n", prefix, check.Name, check.Detail)
	}
	if failed {
		return ExitRuntime
	}
	return ExitOK
}
