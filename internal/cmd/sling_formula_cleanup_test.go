package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestRunSlingFormulaCleansDelayedDogFailure(t *testing.T) {
	data, err := os.ReadFile("sling_formula.go")
	if err != nil {
		t.Fatalf("read sling_formula.go: %v", err)
	}
	source := string(data)

	funcStart := strings.Index(source, "func runSlingFormula(")
	if funcStart == -1 {
		t.Fatal("runSlingFormula not found")
	}
	body := source[funcStart:]
	nextFunc := strings.Index(body[1:], "\nfunc ")
	if nextFunc != -1 {
		body = body[:nextFunc+1]
	}

	for _, want := range []string{
		") (err error)",
		"defer func()",
		"cleanupFailedDogFormulaWisp(wispRootID, formulaWorkDir)",
		"delayedDogInfo.clearWorkIfMatches()",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runSlingFormula missing %q", want)
		}
	}

	cleanupIdx := strings.Index(body, "cleanupFailedDogFormulaWisp(wispRootID, formulaWorkDir)")
	clearIdx := strings.Index(body, "delayedDogInfo.clearWorkIfMatches()")
	if cleanupIdx == -1 || clearIdx == -1 || cleanupIdx > clearIdx {
		t.Fatal("runSlingFormula must close the wisp before clearing dog work")
	}
}

func TestRunSlingFormulaDogNudgeBeforeEmptyPaneReturn(t *testing.T) {
	data, err := os.ReadFile("sling_formula.go")
	if err != nil {
		t.Fatalf("read sling_formula.go: %v", err)
	}
	source := string(data)

	dogNudgeIdx := strings.LastIndex(source, "nudgeFormulaDog(delayedDogInfo, prompt)")
	emptyPaneIdx := strings.Index(source, "if targetPane == \"\" {")
	if dogNudgeIdx == -1 {
		t.Fatal("dog-specific nudge call not found")
	}
	if emptyPaneIdx == -1 {
		t.Fatal("empty-pane return block not found")
	}
	if dogNudgeIdx > emptyPaneIdx {
		t.Fatal("dog-specific nudge must run before generic empty-pane return")
	}
}
