package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/spf13/cobra"
)

// This version variable is set at compile time using ldflags
var version = "dev"

type CommitMessageResponse struct {
	CommitMessage string `json:"commit_message"`
}

var CommitMessageResponseSchema = GenerateSchema[CommitMessageResponse]()

func GenerateSchema[T any]() any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return schema
}

// get_git_crypt_files returns the list of files that are encrypted with
// git-crypt (i.e. have the "filter=git-crypt" attribute set in .gitattributes).
// When a git-crypt repository is unlocked, `git diff` emits the decrypted
// plaintext of these files, so they must be excluded from anything sent to the
// LLM to avoid leaking secrets.
func get_git_crypt_files() ([]string, error) {
	// List all tracked files, then ask git which of them carry the
	// git-crypt filter attribute.
	lsCmd := exec.Command("git", "ls-files", "-z")
	lsOutput, err := lsCmd.Output()
	if err != nil {
		return nil, err
	}

	files := strings.Split(strings.TrimRight(string(lsOutput), "\x00"), "\x00")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		return nil, nil
	}

	// `git check-attr --stdin -z filter` reads NUL-separated paths on stdin
	// and emits NUL-separated triplets: <path>\0<attr>\0<value>\0
	attrCmd := exec.Command("git", "check-attr", "--stdin", "-z", "filter")
	attrCmd.Stdin = strings.NewReader(strings.Join(files, "\x00"))
	attrOutput, err := attrCmd.Output()
	if err != nil {
		return nil, err
	}

	fields := strings.Split(strings.TrimRight(string(attrOutput), "\x00"), "\x00")
	var crypted []string
	for i := 0; i+2 < len(fields); i += 3 {
		path, value := fields[i], fields[i+2]
		if strings.HasPrefix(value, "git-crypt") {
			crypted = append(crypted, path)
		}
	}

	return crypted, nil
}

func run_git_diff() (string, error) {
	cryptFiles, err := get_git_crypt_files()
	if err != nil {
		return "", err
	}

	// Build pathspec exclusions so git-crypt encrypted files never appear in
	// the diff (their decrypted contents could contain secrets). Git requires
	// an inclusive pathspec ("." here) alongside any :(exclude) pathspecs,
	// otherwise nothing matches.
	var pathspecs []string
	if len(cryptFiles) > 0 {
		pathspecs = append(pathspecs, ".")
		for _, f := range cryptFiles {
			pathspecs = append(pathspecs, ":(exclude)"+f)
		}
	}

	// First check if there are staged files
	stagedArgs := []string{"diff", "--staged", "--name-only", "--"}
	stagedArgs = append(stagedArgs, pathspecs...)
	stagedCmd := exec.Command("git", stagedArgs...)
	stagedOutput, err := stagedCmd.Output()
	if err != nil {
		return "", err
	}

	// If there are staged files, diff only staged files
	if len(stagedOutput) > 0 {
		args := []string{"diff", "--staged", "HEAD", "--"}
		args = append(args, pathspecs...)
		cmd := exec.Command("git", args...)
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return string(output), nil
	}

	// Otherwise, diff unstaged files
	args := []string{"diff", "HEAD", "--"}
	args = append(args, pathspecs...)
	cmd := exec.Command("git", args...)
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

var (
	model    string
	endpoint string
	apiKey   string

	rootCmd = &cobra.Command{
		Use:   "conventional-commits-ai",
		Short: "Generate conventional commit messages with AI",
		Run: func(cmd *cobra.Command, args []string) {
			openaiApiKey := os.Getenv("OPENAI_API_KEY")
			if apiKey == "" && openaiApiKey != "" {
				apiKey = openaiApiKey
			}

			if endpoint == "" && apiKey == "" {
				fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is not set (and you are in OpenAI mode)")
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

			clientOptions := []option.RequestOption{option.WithAPIKey(apiKey)}
			if endpoint != "" {
				clientOptions = append(clientOptions, option.WithBaseURL(endpoint))
			}
			client := openai.NewClient(clientOptions...)

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
	- do not contain a scope, then DO NOT use a scope in the commit message (omit it entirely).
	- contain a ticket number or pull request number, then the scope should be the ticket number or pull request number.

	<subject> is a short description of the change.

	IMPORTANT: Do NOT geneate the body of the commit message. Only the subject.
	`

			schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "commit_message",
				Description: openai.String("Commit message"),
				Schema:      CommitMessageResponseSchema,
				Strict:      openai.Bool(true),
			}

			chat, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
				Model: model,
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
)

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
	rootCmd.Flags().StringVarP(&model, "model", "m", "gpt-4.1", "Model to use (e.g., gpt-4.1, llama3.2, etc.)")
	rootCmd.Flags().StringVarP(&endpoint, "endpoint", "e", "", "Custom API endpoint (e.g., http://localhost:11434/v1 for Ollama)")
	rootCmd.Flags().StringVarP(&apiKey, "apikey", "k", "", "API key for custom endpoints")
}
