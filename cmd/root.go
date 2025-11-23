package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/spf13/cobra"
)

// This version variable is set at compile time using ldflags
var version = "dev"

const (
	MODEL = openai.ChatModelGPT4_1
)

type CommitMessageResponse struct {
	CommitMessage string `json:"commit_message"`
}

var CommitMessageResponseSchema = GenerateSchema[CommitMessageResponse]()

func GenerateSchema[T any]() interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return schema
}

func run_git_diff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func run_git_log() (string, error) {
	cmd := exec.Command("git", "log", "--pretty=format:%s", "-n", "30")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func run_git_branch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

var rootCmd = &cobra.Command{
	Use:   "conventional-commits-ai",
	Short: "Generate conventional commit messages with AI",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is not set")
			os.Exit(1)
		}

		gitDiff, err := run_git_diff()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run git diff:", err)
			os.Exit(1)
		}

		gitLog, err := run_git_log()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run git log:", err)
			os.Exit(1)
		}

		gitBranch, err := run_git_branch()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run git branch:", err)
			os.Exit(1)
		}

		client := openai.NewClient(option.WithAPIKey(apiKey))

		systemPrompt := `You are a commit message generator.
	You will be given a git diff of the current changes, a log of the last commits, and the current branch name.
	Your job is to generate a commit message that is as short as possible but as descriptive as possible.
	The generated commit message will be similar to the convention of the last commit messages (in terms of type, scope, subject)
	and MUST be in the following format (conventional commits):
	<type>(<scope>): <subject>

	<type> can be one of the following:
	- feat: A new feature
	- fix: A bug fix
	- docs: Documentation only changes
	- style: Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons, etc)
	- refactor: A code change that neither fixes a bug nor adds a feature
	- perf: A code change that improves performance
	- test: Adding missing tests or correcting existing tests
	- build: Changes that affect the build system or external dependencies (example scopes: gulp, broccoli, npm)
	- ci: Changes to our CI configuration files and scripts (example scopes: Travis, Circle, BrowserStack, SauceLabs)
	- chore: Other changes that don't modify src or test files
	- revert: Reverts a previous commit

	<scope> is optional and can be anything specifying the place of the commit change.
	If the git log contains previous examples of conventional commits, the scope should follow the pattern of the previous commits.
	If the previous similar commits:
	- do not contain a scope, then the scope should be the type of the commit.
	- contain a ticket number or pull request number, then the scope should be the ticket number or pull request number.

	<subject> is a short description of the change.
	`

		schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:        "commit_message",
			Description: openai.String("Commit message"),
			Schema:      CommitMessageResponseSchema,
			Strict:      openai.Bool(true),
		}

		chat, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
			Model: MODEL,
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(fmt.Sprintf("Current branch: %s", gitBranch)),
				openai.UserMessage(fmt.Sprintf("Git diff:\n%s", gitDiff)),
				openai.UserMessage(fmt.Sprintf("Git log:\n%s", gitLog)),
			},
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
					JSONSchema: schemaParam,
				},
			},
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to create chat completion:", err)
			os.Exit(1)
		}

		var commitMessageResponse CommitMessageResponse
		if err := json.Unmarshal([]byte(chat.Choices[0].Message.Content), &commitMessageResponse); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to unmarshal commit message response:", err)
			os.Exit(1)
		}

		fmt.Println(commitMessageResponse.CommitMessage)

	},
}

func Execute() {
	// If the version is not set at compile time, try to get it from the build info
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	rootCmd.Version = version

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.conventional-commits-ai.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
