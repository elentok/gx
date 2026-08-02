package git

import (
	"encoding/json"
	"fmt"
)

// graphQLSearchEnvelope is the `{"data":{"search":{"nodes":[...]}}}` shape
// gh api graphql returns for both openPRsSearchQuery and
// closedPRsSearchQuery.
type graphQLSearchEnvelope[T any] struct {
	Errors graphQLErrors `json:"errors"`
	Data   struct {
		Search struct {
			Nodes []T `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
}

// graphQLErrors is the `errors` array `gh api graphql` responses carry
// alongside (or instead of) `data`, shared by every envelope shape so the
// "any errors? report the first one" check isn't repeated per envelope.
type graphQLErrors []struct {
	Message string `json:"message"`
}

func (e graphQLErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	return fmt.Errorf("gh api graphql: %s", e[0].Message)
}

// runGraphQLRequest runs `gh api graphql` with the given query plus extra
// `-f`/`-F` flag args (e.g. `"-f", "searchQuery=..."` or `"-F",
// "number=5"`), decoding the JSON response into target. Shared by
// runGraphQLPRSearch and FetchPRComments so the "shell out to gh, then
// decode" mechanics aren't duplicated per query shape.
func runGraphQLRequest(dir, query string, extraArgs []string, target any) error {
	args := append([]string{"api", "graphql", "-f", "query=" + query}, extraArgs...)
	out, err := runGH(dir, args)
	if err != nil {
		return classifyPRListError(err)
	}
	if err := json.Unmarshal([]byte(out), target); err != nil {
		return fmt.Errorf("parsing gh api graphql response: %w", err)
	}
	return nil
}

// runGraphQLPRSearch runs a search-shaped GraphQL query with the given
// $searchQuery variable, decoding the resulting nodes as T (either
// prSearchNode or closedPRSearchNode). Shared by listOpenPRsAllRepos and
// listClosedPRsAllRepos so the "one GraphQL search query, across every repo"
// shape isn't duplicated for each PR kind.
func runGraphQLPRSearch[T any](dir, query, searchQuery string) ([]T, error) {
	var envelope graphQLSearchEnvelope[T]
	if err := runGraphQLRequest(dir, query, []string{"-f", "searchQuery=" + searchQuery}, &envelope); err != nil {
		return nil, err
	}
	if err := envelope.Errors.err(); err != nil {
		return nil, err
	}
	return envelope.Data.Search.Nodes, nil
}
