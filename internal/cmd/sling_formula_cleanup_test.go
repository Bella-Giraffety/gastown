package cmd

import (
	"os"
	"strings"
	"testing"
)

func runSlingFormulaSourceForTest(t *testing.T) string {
	t.Helper()
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
	return body
}

func TestRunSlingFormulaCleansDelayedDogFailure(t *testing.T) {
	body := runSlingFormulaSourceForTest(t)

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

	unlockDeferIdx := strings.Index(body, "defer assigneeUnlock()")
	cleanupDeferIdx := strings.Index(body, "defer func()")
	if unlockDeferIdx == -1 || cleanupDeferIdx == -1 || unlockDeferIdx > cleanupDeferIdx {
		t.Fatal("dog formula cleanup must be deferred after assignee unlock so it runs before unlocking")
	}
	cleanupBody := body[cleanupDeferIdx:]
	cleanupIdx := strings.Index(cleanupBody, "cleanupFailedDogFormulaWisp(wispRootID, formulaWorkDir)")
	clearIdx := strings.Index(cleanupBody, "delayedDogInfo.clearWorkIfMatches()")
	if cleanupIdx == -1 || clearIdx == -1 || cleanupIdx > clearIdx {
		t.Fatal("runSlingFormula must close the wisp before clearing dog work")
	}
}

func TestRunSlingFormulaExistingHookedDogStartsDelayedSession(t *testing.T) {
	body := runSlingFormulaSourceForTest(t)

	existingIdx := strings.Index(body, "existing != nil && !slingForce")
	if existingIdx == -1 {
		t.Fatal("existing hooked formula no-op block not found")
	}
	existingBlock := body[existingIdx:]
	stepIdx := strings.Index(existingBlock, "\n\t// Step 1:")
	if stepIdx == -1 {
		t.Fatal("could not isolate existing hooked formula block")
	}
	existingBlock = existingBlock[:stepIdx]
	startIdx := strings.Index(existingBlock, "delayedDogInfo.StartDelayedSession()")
	completeIdx := strings.Index(existingBlock, "delayedDogComplete = true")
	nudgeIdx := strings.Index(existingBlock, "nudgeFormulaDog(delayedDogInfo, formulaSlingPrompt(formulaName))")
	returnIdx := strings.LastIndex(existingBlock, "return nil")
	if startIdx == -1 {
		t.Fatal("existing hooked formula path must start the delayed dog session")
	}
	if completeIdx == -1 || completeIdx < startIdx {
		t.Fatal("existing hooked formula path must mark delayed dog startup complete")
	}
	if nudgeIdx == -1 || nudgeIdx < completeIdx {
		t.Fatal("existing hooked formula path must nudge the dog before returning")
	}
	if returnIdx != -1 && returnIdx < nudgeIdx {
		t.Fatal("existing hooked formula path returns before starting/nudging dog")
	}
}

func TestRunSlingFormulaDogNudgeBeforeEmptyPaneReturn(t *testing.T) {
	body := runSlingFormulaSourceForTest(t)

	dogNudgeIdx := strings.LastIndex(body, "nudgeFormulaDog(delayedDogInfo, prompt)")
	emptyPaneIdx := strings.Index(body, "if targetPane == \"\" {")
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
