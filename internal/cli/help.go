// 本文件维护命令帮助文本与帮助路由。
package cli

import "strings"

// helpTexts maps a command path to its localized usage text.
var helpTexts = map[string]string{
	"cert":                "help.cert",
	"cert list":           "help.cert.list",
	"cert expiring":       "help.cert.expiring",
	"cert verify":         "help.cert.verify",
	"cert issue":          "help.cert.issue",
	"cert reissue":        "help.cert.reissue",
	"cert revoke":         "help.cert.revoke",
	"cert unrevoke":       "help.cert.unrevoke",
	"cert trash":          "help.cert.trash",
	"cert export":         "help.cert.export",
	"cert platform":       "help.cert.platform",
	"cert platform add":   "help.cert.platform.add",
	"cert platform set":   "help.cert.platform.set",
	"cert package":        "help.cert.package",
	"pki":                 "help.pki",
	"pki index":           "help.pki.index",
	"pki index sync":      "help.pki.index.sync",
	"pki downloads":       "help.pki.downloads",
	"pki downloads list":  "help.pki.downloads.list",
	"pki downloads clean": "help.pki.downloads.clean",
	"trash":               "help.trash",
	"trash list":          "help.trash.list",
	"trash restore":       "help.trash.restore",
	"trash purge":         "help.trash.purge",
	"web":                 "help.web",
	"web status":          "help.web.status",
	"web readonly":        "help.web.readonly",
	"web idle-timeout":    "help.web.idle-timeout",
	"web export":          "help.web.export",
	"web lock":            "help.web.lock",
	"web lockdown":        "help.web.lockdown",
	"web hash-password":   "help.web.hash-password",
	"config":              "help.config",
	"config init":         "help.config.init",
	"config show":         "help.config.show",
	"config validate":     "help.config.validate",
	"config export-env":   "help.config.export-env",
	"config path":         "help.config.path",
	"wizard":              "help.wizard",
	"completion":          "help.completion",
	"cert import":         "help.cert.import",
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func resolveHelpPath(args []string) string {
	best := ""
	path := ""
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if path == "" {
			path = arg
		} else {
			path += " " + arg
		}
		if _, ok := helpTexts[path]; ok {
			best = path
		}
	}
	return best
}
