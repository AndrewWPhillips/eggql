# Go GraphQL packages

## Performance Comparison

To compare the performance of the packages I wrote the same random number generator in each (see [Source Code](#source-code)).  I compared the average response time over 100,000 requests using jMeter.

Note that this is a very simple test using in-memory data.  With a production GraphQL server based on a database the results would undoubtedly be different.  Also, if your root query is big (ie has a lot of queries) **eggql** would be a lot slower than some others as it currently does a linear search of the fields of the root query object.  This will probably be addressed before the release of version 1.0.

I ran the tests using this query:

```graphql
{
	"query": "{ random }"
}
```
except that for **graphql-go** the default argument values do not work (bug?) and for **thunder** I could not work out how to specify defaults for arguments (not implemented?).  If you know how to address these problems please tell me.

To test these packages I instead used the query below that does not rely on default values for arguments.  This may have affected performance but I doubt by very much.


```graphql
{
	"query": "{ random(low:1, high:6) }"
}
```

The results were surprising, especially as **gqlgen** is 6 times slower (at about 1.5 milliseconds per request) than **graphql-go** and **eggql***.  I suspect there is something wrong with my code and/or testing strategy.


|                          Project                          |        By        | Time (msec) | Lines of Code | Written LOC |
|:---------------------------------------------------------:|:----------------:|:-----------:|--------------:|------------:|
|     [graphql](https://github.com/graphql-go/graphql)      |    graphql-go    |    0.62     |            61 |          40 |
| [graphql-go](https://github.com/graph-gophers/graphql-go) |  graph-gophers   |    0.24     |            28 |          11 |
|       [gqlgen](https://github.com/99designs/gqlgen)       |    99 Designs    |    1.52     |         >2500 |           6 |
|      [thunder](https://github.com/samsarahq/thunder)      | Samsara Networks |    1.60     |            21 |           9 |
|     [eggql](https://github.com/AndrewWPhillips/eggql)     |   This project   |    0.28     |            14 |           4 |

## Code Comparison

In all cases I took (or generated for gqlgen) a simple example project and removed unneeded code specific to that example.  Then I added/modified the required lines of code (see **Written LOC** in the above table).  However, for **thunder** I greatly simplified their "Getting Started" example (halving the code size) to make the comparison fairer.

The **eggql** solution gave the smallest program (and, I believe, simplest to write). It was 14 lines of code in total.  I copied the code from another simple project and modified 4 lines.

**gqlgen** was next best.  Although it generated thousands of lines of code, it only required adding 6 lines of code (3 line schema + 3 line Go function).  However, it took me more than an hour to get the project set up and working, even though this is not the first gqlgen project I have created!

In fact all the programs, apart from the one using **eggql**, took hour(s) to write and get working (but maybe I am a bit slow:).  And, yes, I have previously created at least one other test project using each of these packages.

I had the **eggql** project compiled and running in just a few minutes.  Please get back to me if your experience differs.

# Source Code

## graphql

```go
package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"

	"github.com/graphql-go/graphql"
)

const JSONContentType = "application/json"

type GraphQLPayload struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func main() {
	// Schema
	fields := graphql.Fields{
		"random": &graphql.Field{
			Type: graphql.Int,
			Args: graphql.FieldConfigArgument{
				"low": {
					Type:         graphql.Int,
					DefaultValue: 1,
				},
				"high": {
					Type:         graphql.Int,
					DefaultValue: 6,
				},
			},
			Resolve: func(params graphql.ResolveParams) (interface{}, error) {
				low := params.Args["low"].(int)
				high := params.Args["high"].(int)
				return low + rand.Intn(high+1-low), nil
			},
		},
	}
	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		log.Fatalf("failed to create new schema, error: %v", err)
	}

	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", JSONContentType)
		// decode the request
		var payload GraphQLPayload
		json.NewDecoder(r.Body).Decode(&payload)

		// encode the response
		json.NewEncoder(w).Encode(graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  payload.Query,
			VariableValues: payload.Variables,
		}))
	})
	http.ListenAndServe(":8083", nil)
}
```

## graphql-go

```go
package main

import (
	"log"
	"math/rand"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

type query struct{}

var schema = `
type Query {
  random(low: Int! = 1, high: Int! = 6): Int!
}
`

func (_ *query) Random(args struct{ Low, High int32 }) int32 {
	return int32(rand.Intn(int(args.High+1-args.Low))) + args.Low
}

func main() {
	schema := graphql.MustParseSchema(schema, &query{})
	http.Handle("/graphql", &relay.Handler{Schema: schema})
	log.Fatal(http.ListenAndServe(":8083", nil))
}
```

## gqlgen

Note that this is not the complete code, but includes the manually added changes.  (The complete code is auto-generated and is more than 2,500 lines of code.)

```graphql
type Query {
  random(low: Int! = 1, high: Int! = 6): Int!
}
```

```Go
package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"math/rand"
	"rand-gqlgen/graph/generated"
)

func (r *queryResolver) Random(ctx context.Context, low int, high int) (int, error) {
	return rand.Intn(high-low) + low, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
```

## thunder


```go
package main

import (
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"math/rand"
	"net/http"
)

func main() {
	builder := schemabuilder.NewSchema()
	obj := builder.Query()
	obj.FieldFunc("random", func(args struct{ Low, High int64 }) int {
		return int(args.Low) + rand.Intn(int(args.High-args.Low+1))
	})
	schema := builder.MustBuild()
	introspection.AddIntrospectionToSchema(schema)
	http.Handle("/graphql", graphql.HTTPHandler(schema))
	http.ListenAndServe(":8083", nil)
}
```

## eggql

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"math/rand"
	"net/http"
)

func main() {
	http.Handle("/graphql", eggql.MustRun(struct {
		Random func(int, int) int `egg:"(low=1,high=6)"`
	}{func(low, high int) int { return low + rand.Intn(high+1-low) }}))
	http.ListenAndServe(":8083", nil)
}
```
