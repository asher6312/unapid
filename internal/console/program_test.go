package console

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/asher6312/unapid/internal/dockerctl"
)

func TestAskYesNoHonorsDefaultsAndRetries(t *testing.T) {
	var output bytes.Buffer
	answer, err := AskYesNo(bufio.NewReader(strings.NewReader("maybe\n\n")), &output, "Continue?", true)
	if err != nil || !answer {
		t.Fatalf("answer=%v err=%v", answer, err)
	}
	answer, err = AskYesNo(bufio.NewReader(strings.NewReader("n\n")), &output, "Continue?", true)
	if err != nil || answer {
		t.Fatalf("answer=%v err=%v", answer, err)
	}
}

func TestChooseContainerSkipsSingleAndValidatesMultiple(t *testing.T) {
	containers := []dockerctl.Container{{Name: "n8n-primary"}, {Name: "n8n-secondary"}}
	var output bytes.Buffer
	choice, err := ChooseContainer(bufio.NewReader(strings.NewReader("bad\n2\n")), &output, containers)
	if err != nil || choice.Name != "n8n-secondary" {
		t.Fatalf("choice=%#v err=%v", choice, err)
	}
	choice, err = ChooseContainer(bufio.NewReader(strings.NewReader("")), &output, containers[:1])
	if err != nil || choice.Name != "n8n-primary" {
		t.Fatalf("single choice=%#v err=%v", choice, err)
	}
}
