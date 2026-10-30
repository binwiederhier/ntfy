# AI policy
I use AI tools to help develop ntfy. Here's how they fit into my work, and what I expect from contributors who use them.

## How I use AI
I use tools like Claude and Codex to write code for ntfy, and **I review every line before it's merged and released**.
If anything, I review code more thoroughly now than I did before. I also write more unit and integration tests, since AI
makes them easier to put together.

For larger changes, I usually read through the diff several times and ask the AI to put together before-and-after
comparisons to help me check the logic. I also run manual regression tests and load tests on a staging server before
releasing those changes on ntfy.sh.

[PostgreSQL support](https://github.com/binwiederhier/ntfy/issues/1114), added in v2.18.0, is a good example. AI wrote the
code, and I spent weeks reviewing and testing it before release. I've shared more about that process in
[this discussion](https://github.com/binwiederhier/ntfy/issues/1645).

AI writes a lot of the code, and I'm still responsible for every line of it.

## AI-assisted pull requests
**AI-assisted pull requests are welcome**, but please **don't make me the first human to read your code**. Before you
submit a pull request, review the code, make sure you understand it, and test it yourself. Don't pass along whatever
your AI tool produced and leave the review to me. **[Don't meat proxy me!](https://nomeatproxy.com/)**

The same applies to issues, pull request descriptions, and comments. Please write them yourself, or at least read and
edit any AI-generated text before posting it. Keep it concise and make sure it says what you actually want to say.

Pull requests that appear to have been generated and submitted without human review will be closed.
