# EGGQL

The eggql package allows you to very easily create a GraphQL server using Go.

It currently supports queries, mutations, all GraphQL types including interfaces. It does not support subscriptions and introspection (yet).

It does not require creating a GraphQL schema. You just declare Go structs with fields that implement the GraphQL resolvers.  For some things, like resolver argument names, you need to add tags (field metadata), similar to the way tags are used for encoding of JSON.

## Getting Started

To create a GraphQL server you must declare a struct, representing the root query object.  Each exported field (ie, having a capitalized name) of this struct represents a query.  Each field can be

- a scalar type (int, string, etc) that represents a GraphQL scalar (Int!, String!, etc)
- a nested struct that represents a GraphQL nested query
- a slice that represents a GraphQL list
- a pointer to one of the above types, in which case the value is nullable

A field can also be a function that *returns* one of the above types.  Using a function as a GraphQL resolver allows it to take arguments.  We shall see a resolver that takes two arguments in the example below.

To start the server you just need to call `eggql.MustRun()` passing an instance of the root query struct.  (You can also pass a mutation struct as we will see later.)  `MustRun()` returns an `http.Handler` which can be used like any other handler with the standard `net/http` package. In the example below we use a path of `/graphql` and port `8080`, which means the queries can be sent locally to the address `http://localhost:8080/graphql`.

Note that the **Must** part of `MustRun()` indicates that no errors are returned - ie, it panics if anything goes wrong.  (We will see later how you can instead get errors returned if something is misconfigured.)  Importantly, it will only panic on configuration problems (ie,  bugs in your code :) which are detected at startup. Once the server is running any errors, such as problems with the incoming query, are diagnosed and returned as part of the query response.

### Example

Here is a simple GraphQL server that returns random integers within a range.  The range defaults to 1 to 6, representing the sides of dice.

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"math/rand"
	"net/http"
)

func main() {
	http.Handle("/graphql", eggql.MustRun(
		struct {
			Random func(low, high int) int `graphql:",params(low=1,high=6)"`
		}{
			func(low, high int) int { return low + rand.Intn(high+1-low) },
		}))
	http.ListenAndServe(":8080", nil)
}
```

To test the server just send a query like the following to http://localhost:8080/graphql

```
{
    random
}
```

I usually test GraphQL servers using Postman (see below) but you can just use curl to post a GraphQL query like this:

```sh
$ curl -XPOST -d '{"query": "{ random }"}' localhost:8080/graphql
```

and you should get a response like this:

```json
{
    "data": {
        "random": 5
    },
    "errors": null
}
```

### Testing with Postman

To use Postman for testing your server just create a new **POST** request using an address of `http://localhost:8080/graphql`. Under the **Headers** tab ensure that `ContentType` is `application/json`.  Under **Body** select **GraphQL** and enter this query:

```
{
    random(high:1000)
}
```

Each time you click the **Send** button you should see a new number between 1 and 1000 like this:

```json
{
    "data": {
        "random": 467
    },
    "errors": null
}
```

### Query Errors

Try this to see what happens when the query has an error:

```sh
$ curl -XPOST -d '{"query": "{ rnd }"}' localhost:8080/graphql
```

The eggql package automatically detects the problems and returns an error response like this:

```json
{
    "errors": [
        {
            "message": "Cannot query field \"rnd\" on type \"Query\". Did you mean \"random\"?",
            "locations": [ { "line": 1, "column": 3 }]
        }
    ]
}
```

