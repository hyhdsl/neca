package helper

import (
	"context"
	"errors"

	"github.com/hyhdsl/neca/internal/core"
)

type operationFunc func(ctx context.Context, server *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error)

var operations = map[Op]operationFunc{
	OpIssue: func(ctx context.Context, s *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error) {
		return core.IssueCertificate(ctx, s.cfg, s.store, s.oss, issueReq)
	},
	OpReissue: func(ctx context.Context, s *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error) {
		return core.ReissueCertificate(ctx, s.cfg, s.store, s.oss, s.engine, issueReq)
	},
	OpRevoke: func(ctx context.Context, s *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error) {
		return core.RevokeCertificate(ctx, s.cfg, s.store, s.oss, s.engine, req.CN, false)
	},
	OpUnrevoke: func(ctx context.Context, s *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error) {
		return core.UnrevokeCertificate(ctx, s.cfg, s.store, s.oss, s.engine, req.CN, false)
	},
}

func supportedOperation(op Op) bool {
	_, ok := operations[op]
	return ok
}

func executeOperation(ctx context.Context, s *Server, req Request, issueReq core.IssueRequest) (*core.CertificateResult, error) {
	execute, ok := operations[req.Op]
	if !ok {
		return nil, errors.New("unsupported op")
	}
	return execute(ctx, s, req, issueReq)
}
