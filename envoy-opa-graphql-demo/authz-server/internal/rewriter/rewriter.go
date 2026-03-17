package rewriter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/parser"
)

// RewriteBody 解析 HTTP body (JSON)，过滤 GraphQL query 中的指定字段，返回改写后的 body。
// deniedFields 中的字段名将从 query 的所有 SelectionSet 中移除。
func RewriteBody(body []byte, deniedFields []string) ([]byte, error) {
	if len(deniedFields) == 0 {
		return body, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal body: %w", err)
	}

	queryRaw, ok := payload["query"]
	if !ok {
		return body, nil
	}
	query, ok := queryRaw.(string)
	if !ok {
		return body, nil
	}

	rewritten, err := removeFields(query, deniedFields)
	if err != nil {
		return nil, fmt.Errorf("remove fields: %w", err)
	}

	payload["query"] = rewritten
	return json.Marshal(payload)
}

// removeFields 从 GraphQL query 字符串中移除指定字段。
func removeFields(query string, deniedFields []string) (string, error) {
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return "", fmt.Errorf("parse query: %w", err)
	}

	denied := make(map[string]struct{}, len(deniedFields))
	for _, f := range deniedFields {
		denied[f] = struct{}{}
	}

	for _, op := range doc.Operations {
		op.SelectionSet = filterSelectionSet(op.SelectionSet, denied)
	}
	for _, frag := range doc.Fragments {
		frag.SelectionSet = filterSelectionSet(frag.SelectionSet, denied)
	}

	var buf strings.Builder
	formatter.NewFormatter(&buf).FormatQueryDocument(doc)
	return strings.TrimSpace(buf.String()), nil
}

func filterSelectionSet(set ast.SelectionSet, denied map[string]struct{}) ast.SelectionSet {
	if len(set) == 0 {
		return set
	}

	filtered := make(ast.SelectionSet, 0, len(set))
	for _, sel := range set {
		switch node := sel.(type) {
		case *ast.Field:
			if _, ok := denied[node.Name]; ok {
				continue
			}
			if len(node.SelectionSet) > 0 {
				node.SelectionSet = filterSelectionSet(node.SelectionSet, denied)
			}
			filtered = append(filtered, node)
		case *ast.InlineFragment:
			node.SelectionSet = filterSelectionSet(node.SelectionSet, denied)
			if len(node.SelectionSet) > 0 {
				filtered = append(filtered, node)
			}
		case *ast.FragmentSpread:
			filtered = append(filtered, node)
		default:
			filtered = append(filtered, node)
		}
	}
	return filtered
}
