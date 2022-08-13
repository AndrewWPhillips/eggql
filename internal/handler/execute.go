package handler

// execute.go handles the execution of a GraphQL request

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
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
		// Data stores the results of the query or queries
		// We use a jsonmap.Ordered rather than a map[string]interface{} so as to remember the order since
		// the query result should have the same order as the query.  A nested query result is stored
		// as a jsonmap.Ordered (as interface{}) within the Data whereas a list is stored as a slice.
		Data   jsonmap.Ordered `json:"data,omitempty"`
		Errors gqlerror.List   `json:"errors,omitempty"`
	}
)

// Execute parses and runs the request (Query field) and returns the result
func (g *gqlRequest) Execute(ctx context.Context) (r gqlResult) {
	// Get the analysed and validated query from the query text
	query, errors := gqlparser.LoadQuery(g.h.schema, g.Query)
	if errors != nil {
		r.Errors = errors
		return
	}

	// Now process the operation(s)
	r.Data.Data = make(map[string]interface{})
	for _, operation := range query.Operations {
		op := gqlOperation{enums: g.h.enums, enumsReverse: g.h.enumsReverse}

		// Get variables associated with this operation if any
		if len(operation.VariableDefinitions) > 0 {
			var pgqlError *gqlerror.Error
			if op.variables, pgqlError = validator.VariableValues(g.h.schema, operation, g.Variables); pgqlError != nil {
				r.Errors = append(r.Errors, pgqlError)
				continue // skip this op if we can't get the vars
			}
		}

		var data []interface{}
		var isSubscription bool
		switch operation.Operation {
		case ast.Query:
			data = g.h.qData
		case ast.Mutation:
			op.isMutation = true
			data = g.h.mData
		case ast.Subscription:
			isSubscription = true
			data = g.h.subscriptionData
		default:
			panic("unexpected")
		}
		var result jsonmap.Ordered
		var err error
		for _, d := range data {
			result, err = op.GetSelections(ctx, operation.SelectionSet, reflect.ValueOf(d), nil)
			if err != nil || len(result.Order) > 0 {
				break
			}
		}

		// TODO: don't stop on 1st error but record all errors to save the client debug time
		if err != nil {
			r.Errors = append(r.Errors, &gqlerror.Error{
				Message:    err.Error(),
				Extensions: map[string]interface{}{"operation": operation.Name},
			})
			return
		}
		for _, k := range result.Order {
			if _, ok := r.Data.Data[k]; ok {
				r.Errors = append(r.Errors, &gqlerror.Error{
					Message:    fmt.Sprintf("resolver %q in %s has duplicate name", k, operation.Name),
					Extensions: map[string]interface{}{"operation": operation.Name},
				})
				return
			}
			r.Data.Data[k] = result.Data[k]
		}
		r.Data.Order = append(r.Data.Order, result.Order...)
		if len(r.Data.Order) != len(r.Data.Data) {
			panic("map and slice in the jsonmap.Ordered should be the same size")
		}
	}
	return
}
