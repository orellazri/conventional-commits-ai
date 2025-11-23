# conventional-commits-ai

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

````

To directly commit the generated commit message, you can run:

```bash
git commit -m "$(conventional-commits-ai)"
````
