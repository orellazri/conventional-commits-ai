# AI Conventional Commits

A small CLI that uses the OpenAI API to generate short, conventional-commit–style messages for your current Git changes.

The tool looks at:

- `git diff HEAD` (your uncommitted changes)
- The last 30 commit subjects (`git log --pretty=format:%s -n 30`)
- The current branch name (`git branch --show-current`)

It then asks an OpenAI model to propose a concise commit message that matches the existing style in your repo.

## Usage

Install the CLI using `go install`:

```bash
go install github.com/orellazri/conventional-commits-ai@latest
```

Or, download the binary from the [releases page](https://github.com/orellazri/conventional-commits-ai/releases) and put it in your `$PATH` (e.g., `/usr/local/bin`).

From the root of your Git repository, run:

```bash
conventional-commits-ai
```

Example output:

```text
feat(api): add search endpoint
```

To directly commit the generated commit message, you can run:

```bash
git commit -m "$(conventional-commits-ai)"
```

### OpenAI

To use a different model, you can pass the `--model` flag:

```bash
conventional-commits-ai --model gpt-4.1-nano
```

The default model is `gpt-4.1`.

### Custom endpoints

To use a custom endpoint (e.g. Ollama, LM Studio, etc.), you can pass the `--endpoint` flag along with the `--model` flag:

```bash
conventional-commits-ai --endpoint http://localhost:11434/v1 --model openai/gpt-oss-20b
```

The endpoint should be a valid OpenAI-compatible API endpoint, such as `http://localhost:11434/v1` for Ollama.
