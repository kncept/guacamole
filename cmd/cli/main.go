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
	// One stdin scanner shared by the REPL and every foreground user prompt
	// (permission checks, user questions, tool-round limits).
	input := &cliInput{scanner: bufio.NewScanner(os.Stdin)}
	userInput := &cliUserInput{input: input}

	manager := permissions.NewPermissionsManager(gcfg, userInput)
	promptRunner, err := promptrunner.NewPromptRunner(conf, gcfg, resumeID, manager, userInput)
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

// cliUserInput asks every foreground user question on the terminal. It
// shares the stdin scanner with the REPL, so prompts never interleave with
// the next prompt.
type cliUserInput struct {
	input *cliInput
}

// Prompt prints the question and its options, then reads one line from
// stdin. A numbered answer picks that option; any other line is accepted
// only when free text is allowed.
func (u *cliUserInput) Prompt(text string, allowFree bool, options []string) (string, error) {
	fmt.Println(text)
	for i, option := range options {
		fmt.Printf("%d) %s\n", i+1, option)
	}
	if allowFree {
		fmt.Print("Enter a number or type an answer: ")
	} else {
		fmt.Print("Enter a number: ")
	}
	if !u.input.scanner.Scan() {
		return "", fmt.Errorf("stdin closed before the question was answered")
	}
	answer := strings.TrimSpace(u.input.scanner.Text())
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
		return options[n-1], nil
	}
	if allowFree && answer != "" {
		return answer, nil
	}
	return "", fmt.Errorf("%q is not a valid response, pick 1-%d", answer, len(options))
}

// printResumeHint prints the command that continues the session in a later
// run.
func printResumeHint(sessionID string) {
	fmt.Printf("Resume with: guacamole --session %s\n", sessionID)
}
