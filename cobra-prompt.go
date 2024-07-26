package cobraprompt

import (
	"context"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/pkg/term/termios"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vikasrao23/go-prompt"
	"golang.org/x/sys/unix"
)

var fd int

var originalTermios *unix.Termios

// DynamicSuggestionsAnnotation for dynamic suggestions.
const DynamicSuggestionsAnnotation = "cobra-prompt-dynamic-suggestions"

// PersistFlagValuesFlag the flag that will be avaiailable when PersistFlagValues is true
const PersistFlagValuesFlag = "persist-flag-values"

// CobraPrompt given a Cobra command it will make every flag and sub commands available as suggestions.
// Command.Short will be used as description for the suggestion.
type CobraPrompt struct {
	// RootCmd is the start point, all its sub commands and flags will be available as suggestions
	RootCmd *cobra.Command

	// GoPromptOptions is for customize go-prompt
	// see https://github.com/tengteng/go-prompt/blob/master/option.go
	GoPromptOptions []prompt.Option

	// DynamicSuggestionsFunc will be executed if an command has CallbackAnnotation as an annotation. If it's included
	// the value will be provided to the DynamicSuggestionsFunc function.
	DynamicSuggestionsFunc func(annotationValue string, document *prompt.Document) []prompt.Suggest

	// PersistFlagValues will persist flags. For example have verbose turned on every command.
	PersistFlagValues bool

	// ShowHelpCommandAndFlags will make help command and flag for every command available.
	ShowHelpCommandAndFlags bool

	// DisableCompletionCommand will disable the default completion command for cobra
	DisableCompletionCommand bool

	// ShowHiddenCommands makes hidden commands available
	ShowHiddenCommands bool

	// ShowHiddenFlags makes hidden flags available
	ShowHiddenFlags bool

	// AddDefaultExitCommand adds a command for exiting prompt loop
	AddDefaultExitCommand bool

	// OnErrorFunc handle error for command.Execute, if not set print error and exit
	OnErrorFunc func(err error)

	// InArgsParser adds a custom parser for the command line arguments (default: strings.Fields)
	InArgsParser func(args string) []string

	// SuggestionFilter will be uses when filtering suggestions as typing
	SuggestionFilter func(suggestions []prompt.Suggest, document *prompt.Document) []prompt.Suggest
}

// Run will automatically generate suggestions for all cobra commands and flags defined by RootCmd
// and execute the selected commands. Run will also reset all given flags by default, see PersistFlagValues
func (co CobraPrompt) Run() {
	co.RunContext(nil)
}

// RunContext same as Run but with context
func (co CobraPrompt) RunContext(ctx context.Context) {
	if co.RootCmd == nil {
		panic("RootCmd is not set. Please set RootCmd")
	}

	co.prepare()
	var err error
	fd, err = syscall.Open("/dev/tty", syscall.O_RDONLY, 0)
	if err != nil {
		panic(err)
	}
	// get the original settings
	originalTermios, err = termios.Tcgetattr(uintptr(fd))
	if err != nil {
		panic(err)
	}

	p := prompt.New(
		Executor, completer,
		co.GoPromptOptions...,
	)

	p.Run()
}

func Executor(input string) {
	// restore the original settings to allow ctrl-c to generate signal
	if err := termios.Tcsetattr(uintptr(fd), termios.TCSANOW, (*unix.Termios)(originalTermios)); err != nil {
		panic(err)
	}

	if input == "test" {
		ctx, cancel := context.WithCancel(context.Background())
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		go func() {
			select {
			case <-c:
				cancel()
			}
		}()
		go func() {
			defer cancel()
			for { // long task
			}
		}()
		select {
		case <-ctx.Done():
			return
		}
	}
}

func completer(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func parseArgsWithQuotes(input string) []string {
	re := regexp.MustCompile(`"[^"]+"|\S+`)
	matches := re.FindAllString(input, -1)

	var args []string
	for _, match := range matches {
		// Remove surrounding double quotes if present
		if strings.HasPrefix(match, `"`) && strings.HasSuffix(match, `"`) {
			match = match[1 : len(match)-1]
		}
		args = append(args, match)
	}

	return args
}

func (co CobraPrompt) parseArgs(in string) []string {
	if co.InArgsParser != nil {
		return co.InArgsParser(in)
	}

	return parseArgsWithQuotes(in)
}

func (co CobraPrompt) prepare() {
	if co.ShowHelpCommandAndFlags {
		// TODO: Add suggestions for help command
		co.RootCmd.InitDefaultHelpCmd()
	}

	if co.DisableCompletionCommand {
		co.RootCmd.CompletionOptions.DisableDefaultCmd = true
	}

	if co.AddDefaultExitCommand {
		co.RootCmd.AddCommand(&cobra.Command{
			Use:   "exit",
			Short: "Exit prompt",
			Run: func(cmd *cobra.Command, args []string) {
				os.Exit(0)
			},
		})
	}

	if co.PersistFlagValues {
		co.RootCmd.PersistentFlags().BoolP(PersistFlagValuesFlag, "",
			false, "Persist last given value for flags")
	}
}

func findSuggestions(co *CobraPrompt, d *prompt.Document) []prompt.Suggest {
	command := co.RootCmd
	args := strings.Fields(d.CurrentLine())

	if found, _, err := command.Find(args); err == nil {
		command = found
	}

	var suggestions []prompt.Suggest
	persistFlagValues, _ := command.Flags().GetBool(PersistFlagValuesFlag)
	addFlags := func(flag *pflag.Flag) {
		if flag.Changed && !persistFlagValues {
			flag.Value.Set(flag.DefValue)
		}
		if flag.Hidden && !co.ShowHiddenFlags {
			return
		}
		if strings.HasPrefix(d.GetWordBeforeCursor(), "--") {
			suggestions = append(suggestions, prompt.Suggest{Text: "--" + flag.Name, Description: flag.Usage})
		} else if strings.HasPrefix(d.GetWordBeforeCursor(), "-") && flag.Shorthand != "" {
			suggestions = append(suggestions, prompt.Suggest{Text: "-" + flag.Shorthand, Description: flag.Usage})
		}
	}

	command.LocalFlags().VisitAll(addFlags)
	command.InheritedFlags().VisitAll(addFlags)

	if command.HasAvailableSubCommands() {
		for _, c := range command.Commands() {
			if !c.Hidden && !co.ShowHiddenCommands {
				suggestions = append(suggestions, prompt.Suggest{Text: c.Name(), Description: c.Short})
			}
			if co.ShowHelpCommandAndFlags {
				c.InitDefaultHelpFlag()
			}
		}
	}

	annotation := command.Annotations[DynamicSuggestionsAnnotation]
	if co.DynamicSuggestionsFunc != nil && annotation != "" {
		suggestions = append(suggestions, co.DynamicSuggestionsFunc(annotation, d)...)
	}

	if co.SuggestionFilter != nil {
		return co.SuggestionFilter(suggestions, d)
	}

	return prompt.FilterHasPrefix(suggestions, d.GetWordBeforeCursor(), true)
}
