Goal: I want to implement a Go CLI that can read from `git log` and use an OpenAI LLM to help generate release notes.

1. This Go project should have very good readability and be educational. I’m a Go beginner, so I want to learn Go syntax and best practices as much as possible from this project.

2. The CLI should be able to extract changes either:

   * From a range of version tags (e.g. `v2.23.1` to `v2.23.6`), or
   * From a range of commit hashes.
     If the ending tag or hash is not provided, it should default to `HEAD`.

3. I want to fetch PR titles (using the `gh` command or a custom implementation), because PR titles are usually the most meaningful summary. Then, based on each commit and the code changes, the tool should infer what was actually changed.

4. The LLM must be an OpenAI model, and the API key should be read from an environment variable. The prompt should be easy to tweak; putting it into a config file would be ideal.

5. The release notes should be written from the user’s perspective (e.g. project/product manager), focusing first on what behavioral changes these modifications bring. This part of the prompt should also be easy to customize.

6. The release notes should categorize items into: `New Feature`, `Performance Improvement`, `Bug Fix`, and `Internal Changes`. Each item should follow the principle in (5), and end with a suffix like:
   `(PR#<number> <LINK>, Author: <name>)`.

7. At the end of the release notes, there should be a complete list of all PRs, including:

   * PR number and link
   * Original PR title
   * The code modules/files that were changed

8. Finally, the release notes should be saved as a Markdown file.

