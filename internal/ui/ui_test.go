package ui
package ui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thoth-go/thoth-go/internal/ui"
)

// newTestRenderer returns a Renderer and the buffer it writes to.
func newTestRenderer() (*ui.Renderer, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return ui.New(buf), buf
}

func TestSuccess_ContainsMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Success("all good: %s", "loops-001")
	out := buf.String()
	if !strings.Contains(out, "all good: loops-001") {
		t.Errorf("Success output missing message: %q", out)
	}
	// Must contain the tick icon.
	if !strings.Contains(out, "✓") {
		t.Errorf("Success output missing ✓: %q", out)
	}
}

func TestError_ContainsMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Error("something broke: %d", 42)
	out := buf.String()
	if !strings.Contains(out, "something broke: 42") {
		t.Errorf("Error output missing message: %q", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("Error output missing ✗: %q", out)
	}
}

func TestWarning_ContainsMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Warning("watch out")
	out := buf.String()
	if !strings.Contains(out, "watch out") {
		t.Errorf("Warning output missing message: %q", out)
	}
	if !strings.Contains(out, "⚠") {
		t.Errorf("Warning output missing ⚠: %q", out)
	}
}

func TestInfo_ContainsMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Info("fetching topic: %s", "concurrency")
	out := buf.String()
	if !strings.Contains(out, "fetching topic: concurrency") {
		t.Errorf("Info output missing message: %q", out)
	}
}

func TestMuted_ContainsMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Muted("secondary detail")
	if !strings.Contains(buf.String(), "secondary detail") {
		t.Errorf("Muted output missing message")
	}
}

func TestBanner_ContainsText(t *testing.T) {
	r, buf := newTestRenderer()
	r.Banner("Thoth-Go v1.0")
	if !strings.Contains(buf.String(), "Thoth-Go v1.0") {
		t.Errorf("Banner missing text")
	}
}

func TestSectionHeader_ContainsTitle(t *testing.T) {
	r, buf := newTestRenderer()
	r.SectionHeader("Static Analysis")
	out := buf.String()
	if !strings.Contains(out, "Static Analysis") {
		t.Errorf("SectionHeader missing title")
	}
	// Must also print a separator.
	if !strings.Contains(out, "─") {
		t.Errorf("SectionHeader missing separator")
	}
}

func TestLabel_ContainsBadgeAndMessage(t *testing.T) {
	r, buf := newTestRenderer()
	r.Label("TOPIC", "concurrency")
	out := buf.String()
	if !strings.Contains(out, "TOPIC") {
		t.Errorf("Label missing badge")
	}
	if !strings.Contains(out, "concurrency") {
		t.Errorf("Label missing message")
	}
}

func TestTestPass_ContainsName(t *testing.T) {
	r, buf := newTestRenderer()
	r.TestPass("case 1: basic input")
	out := buf.String()
	if !strings.Contains(out, "case 1: basic input") {
		t.Errorf("TestPass missing name")
	}
}

func TestTestFail_ContainsExpectedAndGot(t *testing.T) {
	r, buf := newTestRenderer()
	r.TestFail("case 2: fizzbuzz", "FizzBuzz\n", "Fizzbuzz\n")
	out := buf.String()
	if !strings.Contains(out, "case 2: fizzbuzz") {
		t.Errorf("TestFail missing name")
	}
	if !strings.Contains(out, "expected") {
		t.Errorf("TestFail missing 'expected' label")
	}
	if !strings.Contains(out, "got") {
		t.Errorf("TestFail missing 'got' label")
	}
	if !strings.Contains(out, "FizzBuzz") {
		t.Errorf("TestFail missing expected content")
	}
	if !strings.Contains(out, "Fizzbuzz") {
		t.Errorf("TestFail missing actual content")
	}
}

func TestStaticError_ContainsRuleAndDetail(t *testing.T) {
	r, buf := newTestRenderer()
	r.StaticError("banned import", "os/exec is not allowed")
	out := buf.String()
	if !strings.Contains(out, "banned import") {
		t.Errorf("StaticError missing rule")
	}
	if !strings.Contains(out, "os/exec is not allowed") {
		t.Errorf("StaticError missing detail")
	}
}

func TestSummary_AllPass(t *testing.T) {
	r, buf := newTestRenderer()
	r.Summary(5, 5)
	out := buf.String()
	if !strings.Contains(out, "5/5") {
		t.Errorf("Summary missing ratio")
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("Summary all-pass should contain ✓")
	}
}

func TestSummary_SomeFail(t *testing.T) {
	r, buf := newTestRenderer()
	r.Summary(3, 5)
	out := buf.String()
	if !strings.Contains(out, "3/5") {
		t.Errorf("Summary missing ratio")
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("Summary partial-pass should contain ✗")
	}
}

func TestEachMethod_WritesNewline(t *testing.T) {
	calls := []func(r *ui.Renderer){
		func(r *ui.Renderer) { r.Success("x") },
		func(r *ui.Renderer) { r.Error("x") },
		func(r *ui.Renderer) { r.Warning("x") },
		func(r *ui.Renderer) { r.Info("x") },
		func(r *ui.Renderer) { r.Muted("x") },
	}
	for _, fn := range calls {
		r, buf := newTestRenderer()
		fn(r)
		out := buf.String()
		if !strings.HasSuffix(out, "\n") {
			t.Errorf("output does not end with newline: %q", out)
		}
	}
}
