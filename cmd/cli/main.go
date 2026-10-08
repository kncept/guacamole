package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/debug"
	"github.com/kncept/guacamole/permissions"
	"github.com/kncept/guacamole/promptrunner"
	"github.com/kncept/guacamole/restfulai"
)

var DEBUG bool = false

type Guacamole interface {
}

type guacamole struct {
	TerminationSignal chan int // send a termination with the passed in exit code

	RestfulAI restfulai.RestfulAI
}

// cli is the command line. kong has no flag aliases, so the --ses synonyms
// are separate fields resolved by firstNonEmpty.
var cli struct {
	Ses     string `name:"ses" short:"s" help:"Resume the saved session with this ID (saved under ~/.guac/session/)."`
	Session string `name:"session" help:"Synonym for --ses."`
}

func main() {
	if DEBUG {
		go debug.RunDebug()
	}

	kong.Parse(&cli)
	resumeID := firstNonEmpty(cli.Ses, cli.Session)

	conf, err := config.DefaultConfig()
	// fmt.Printf("%+v\n", conf)
	if err != nil {
		panic(err)
	}

	gcfg, err := config.Load()
	if err != nil {
		fmt.Printf("Could not load config: %v\n", err)
		os.Exit(1)
	}

	replLoop(conf, gcfg, resumeID)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func replLoop(conf *config.ApiModelInterfaceDetails, gcfg *config.GConfig, resumeID string) {
	// One stdin scanner shared by the REPL and permission prompts.
	input := &cliInput{scanner: bufio.NewScanner(os.Stdin)}

	granter := &cliGranter{input: input}
	manager := permissions.NewPermissionsManager(gcfg, granter)
	questionHandler := &cliQuestionHandler{input: input}

	promptRunner, err := promptrunner.NewPromptRunner(conf, gcfg, resumeID, manager, questionHandler)
	if err != nil {
		fmt.Printf("Could not start session: %v\n", err)
		os.Exit(1)
	}

	// Ctrl-C still reports the session on the way out. The session file is
	// already up to date: it is saved after every completed turn.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	go func() {
		<-interrupt
		fmt.Printf("\nInterrupted. Session ID: %s\n", promptRunner.SessionID())
		printResumeHint(promptRunner.SessionID())
		os.Exit(0)
	}()

	for {
		modelLabel := ""
		if conf.ModelName != "" {
			modelLabel = fmt.Sprintf(" (model: %s)", conf.ModelName)
		}
		fmt.Printf("Enter text%s: ", modelLabel)
		if !input.scanner.Scan() {
			// EOF (Ctrl-D): end the session.
			fmt.Println()
			break
		}

		text := strings.TrimSpace(input.scanner.Text())
		if text == "" {
			continue
		}
		if text == "/new" {
			promptRunner.Reset()
			fmt.Printf("(new session: %s)\n", promptRunner.SessionID())
			continue
		}

		if err := promptRunner.RunPrompt(text); err != nil {
			fmt.Printf("Error occurred: %v\n", err)
		}
	}

	if err := input.scanner.Err(); err != nil {
		fmt.Println("Error:", err)
	}

	path, err := promptRunner.Save()
	if err != nil {
		fmt.Printf("Could not save session %s: %v\n", promptRunner.SessionID(), err)
	} else {
		fmt.Printf("Session saved to %s\n", path)
		printResumeHint(promptRunner.SessionID())
	}
}

// cliInput owns the shared stdin scanner used by both the REPL and the
// permission prompts.
type cliInput struct {
	scanner *bufio.Scanner
}

// cliGranter asks the user for a permission decision on the terminal.
type cliGranter struct {
	input *cliInput
}

// AskForAccess prompts the user for a yes/no decision and returns a policy.
// category names the permission category being checked ("filesystem write",
// "shell", "web"); value is the path, command or domain. Anything other than
// y/yes is treated as deny.
func (g *cliGranter) AskForAccess(category string, value string) config.Policy {
	fmt.Printf("Grant %s access to %s? [y/N]: ", category, value)
	if !g.input.scanner.Scan() {
		return config.PolicyDeny
	}
	answer := strings.TrimSpace(g.input.scanner.Text())
	if strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
		return config.PolicyAllow
	}
	return config.PolicyDeny
}

// cliQuestionHandler asks the model's user questions on the terminal. It
// shares the stdin scanner with the REPL and the permission prompts.
type cliQuestionHandler struct {
	input *cliInput
}

// UserQuestionCallback prints the question and its suggested responses, then
// reads one line from stdin. A numbered answer picks that response; any other
// line is accepted only when freetext is allowed.
func (h *cliQuestionHandler) UserQuestionCallback(question string, responses []string, allowFreetext bool) (string, error) {
	fmt.Println(question)
	for i, response := range responses {
		fmt.Printf("%d) %s\n", i+1, response)
	}
	if allowFreetext {
		fmt.Print("Enter a number or type an answer: ")
	} else {
		fmt.Print("Enter a number: ")
	}
	if !h.input.scanner.Scan() {
		return "", fmt.Errorf("stdin closed before the question was answered")
	}
	answer := strings.TrimSpace(h.input.scanner.Text())
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(responses) {
		return responses[n-1], nil
	}
	if allowFreetext && answer != "" {
		return answer, nil
	}
	return "", fmt.Errorf("%q is not a valid response, pick 1-%d", answer, len(responses))
}

// printResumeHint prints the command that continues the session in a later
// run.
func printResumeHint(sessionID string) {
	fmt.Printf("Resume with: guacamole --session %s\n", sessionID)
}
