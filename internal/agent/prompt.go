package agent

import "fmt"

// RunPrompt is the concise, reliable instruction handed to an agent executing a
// ticket. It tells the agent to use the attached Jeera MCP server to load the
// ticket and keep its status updated as it works, so the human's board reflects
// progress live. {KEY} and {TITLE} are the only dynamic parts.
func RunPrompt(key, title string) string {
	return fmt.Sprintf(`You are an autonomous software engineer executing a Jeera ticket. The Jeera MCP server is attached as `+"`jeera`"+`.

Ticket: %s — %s

1. Call jeera.get_issue with key "%s" for the full description, acceptance criteria, and linked issues.
2. Set the status to "In Progress" via jeera.transition_issue now.
3. Implement the work in this repository; make focused commits; verify it builds and tests pass before finishing.
4. Post a short summary via jeera.add_comment, then set the status to "Done". If you cannot finish, set the status to "Blocked" with a comment explaining why.

Be concise and act without asking for confirmation.`, key, title, key)
}

// DiscussPrompt is the preloaded prompt for the interactive "Expand / Discuss"
// handoff: it opens the ticket for a conversation rather than autonomous work.
func DiscussPrompt(key string) string {
	return fmt.Sprintf("Use the jeera MCP server. Load ticket %s with jeera.get_issue and let's expand on it — clarify scope, acceptance criteria and approach. Don't start implementing yet.", key)
}
