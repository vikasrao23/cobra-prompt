package main

import (
	"os"
	"strings"

	cobraprompt "github.com/tengteng/cobra-prompt"
	"github.com/tengteng/cobra-prompt/_example/cmd"
	"github.com/tengteng/go-prompt"
)

var advancedPrompt = &cobraprompt.CobraPrompt{
	RootCmd:                  cmd.RootCmd,
	PersistFlagValues:        true,
	ShowHelpCommandAndFlags:  true,
	DisableCompletionCommand: true,
	AddDefaultExitCommand:    true,
	GoPromptOptions: []prompt.Option{
		prompt.OptionTitle("cobra-prompt"),
		prompt.OptionPrefix(">(^!^)> "),
		prompt.OptionMaxSuggestion(10),
	},
	DynamicSuggestionsFunc: func(annotationValue string, document *prompt.Document) []prompt.Suggest {
		if suggestions := cmd.GetFoodDynamic(annotationValue); suggestions != nil {
			return suggestions
		}

		return []prompt.Suggest{}
	},
	OnErrorFunc: func(err error) {
		if strings.Contains(err.Error(), "unknown command") {
			cmd.RootCmd.PrintErrln(err)
			return
		}

		cmd.RootCmd.PrintErr(err)
		os.Exit(1)
	},
}

var simplePrompt = &cobraprompt.CobraPrompt{
	RootCmd:                  cmd.RootCmd,
	AddDefaultExitCommand:    true,
	DisableCompletionCommand: true,
}

func main() {
	// Change to simplePrompt to see the difference
	advancedPrompt.Run()
}
