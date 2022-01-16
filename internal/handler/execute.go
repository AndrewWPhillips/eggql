package handler

// execute.go handles the execution of a GraphQL request

import (
	"context"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

type (
	// gqlRequest decodes and handles each GraphQL request
	gqlRequest struct {
		h *Handler

		// These are decoded from the http request body (JSON)
		Query         string
		OperationName string
		Variables     map[string]interface{}
	}

	// gqlResult contains the result (or errors) of the request to be encoded in JSON
	gqlResult struct {
		Data   map[string]interface{} `json:"data,omitempty"`
		Errors gqlerror.List          `json:"errors,omitempty"`
	}
)

// Execute parses and runs the request and returns the result
func (g *gqlRequest) Execute(ctx context.Context) (r gqlResult) {
	// First analyse and validate the query string
	query, pgqlError := parser.ParseQuery(&ast.Source{
		Name:  "query",
		Input: g.Query,
	})
	if pgqlError != nil {
		r.Errors = append(r.Errors, pgqlError)
		return
	}

	r.Errors = validator.Validate(g.h.schema, query)
	if r.Errors != nil {
		return
	}

	r.Data = make(map[string]interface{})
	for _, operation := range query.Operations {
		// TODO: check if ctx has expired?
		op := gqlOperation{enums: g.h.enums}

		// Get variables associated with this operation if any
		if len(operation.VariableDefinitions) > 0 {
			if op.variables, pgqlError = validator.VariableValues(g.h.schema, operation, g.Variables); pgqlError != nil {
				r.Errors = append(r.Errors, pgqlError)
				continue // skip this op if we can't get the vars
			}
		}

		var result map[string]interface{}
		var err error
		switch operation.Operation {
		case ast.Query:
			result, err = op.GetSelections(ctx, operation.SelectionSet, g.h.qData)
		case ast.Mutation:
			op.isMutation = true // TODO: run queries (but not mutations) in separate Go routines
			result, err = op.GetSelections(ctx, operation.SelectionSet, g.h.mData)
		case ast.Subscription:
			//panic("TODO")
		default:
			panic("unexpected")
		}

		// TODO: don't stop on 1st error but record all errors to save the client debug time
		if err != nil {
			r.Errors = append(r.Errors, &gqlerror.Error{
				Message:    err.Error(),
				Extensions: map[string]interface{}{"operation": operation.Name},
			})
			return
		}
		for k, v := range result {
			// TODO check if k is already in use?
			r.Data[k] = v
		}
	}
	return
}
