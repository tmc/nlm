/*
Package api provides a client for interacting with the NotebookLM API.

Basic usage:

	client := api.New(authToken, cookies)

	// List projects
	projects, err := client.ListRecentlyViewedProjects()

	// Add a source
	id, err := client.AddSourceFromFile("project-id", "document.pdf",
	    api.WithSourceName("Important Document"),
	    api.WithContentType("application/pdf"),
	    api.WithBase64Encoding(),
	)

The client handles authentication and provides a clean interface for all NotebookLM
operations including project management, source handling, note creation, and content
generation.
*/
package api
