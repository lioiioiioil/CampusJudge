//go:build linux

package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestStdoutAndStderrShareOneHardBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budget := &outputBudget{limit: 5, cancel: cancel}
	_, _ = io.Copy(budget.writer(true), strings.NewReader("abc"))
	_, _ = io.Copy(budget.writer(false), strings.NewReader("defghi"))
	if budget.stdout.String() != "abc" || budget.stderr.String() != "de" || !budget.exceeded || ctx.Err() == nil {
		t.Fatalf("stdout=%q stderr=%q exceeded=%v canceled=%v", budget.stdout.String(), budget.stderr.String(), budget.exceeded, ctx.Err())
	}
}
