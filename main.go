package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/debug"

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
	Ses     string `name:"ses" short:"s" help:"Resume the saved session with this ID (saved under ./sessions/)."`
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

	replLoop(conf, resumeID)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func replLoop(conf *config.ApiModelInterfaceDetails, resumeID string) {
	promptRunner, err := promptrunner.NewPromptRunner(conf, resumeID)
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

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Enter text: ")
		if !scanner.Scan() {
			// EOF (Ctrl-D): end the session.
			fmt.Println()
			break
		}

		text := strings.TrimSpace(scanner.Text())
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

	if err := scanner.Err(); err != nil {
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

// printResumeHint prints the command that continues the session in a later
// run.
func printResumeHint(sessionID string) {
	fmt.Printf("Resume with: guacamole --session %s\n", sessionID)
}
