# EGGQL

The eggql package allows you to very easily create a GraphQL server using Go.

It currently supports queries, mutations, all GraphQL types including interfaces. It does not support subscriptions and introspection (yet).

For simplicity, you don't need to create a GraphQL *schema*. You just declare Go structs with fields that act as the GraphQL *resolvers*.  For some things, like resolver arguments, you need to add tags (metadata attached to a field of a struct type), similar to the way tags are used for encoding of JSON.

## Getting Started

To create a GraphQL server you must declare a struct, representing the root query object.  Each exported field (ie, having a capitalized name) of this struct represents a root query.  A field can be

- a scalar type (int, string, etc) that represents a GraphQL scalar (Int!, String!, etc)
- an integer type (int, int8, uint, etc) that represents an enumeration
- a nested struct that represents a GraphQL nested query
- a slice that represents a GraphQL list of any of the above types
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
			Random func(int, int) int `graphql:",params(low=1,high=6)"`
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

Note that the query name `random` is derived from the struct's field name `Random`.  Only exported fields (those with a upper-case first letter) are used and the generated GraphQL name is the same but with a lower-case first letter.  You can also provide your own name using the graphql tag such as `graphql:"myRand,params(low=1,high=6)"`.

Also note the two resolver arguments (`low` and `high`) given in the graphql tag.  You must supply the `params` option of the tag if the resolver function takes arguments.  In this case there are two arguments so you must specify two names in the `params`.  (An exception is if the first function argument is a `context.Context` as we will see below.)

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

To use Postman for testing your server just create a new **POST** request using an address of `http://localhost:8080/graphql`. (This address comes from the values in the call to `http.ListenAndServe()` and `http.Handle()` in the Go code above.)

In Postman under the address select the **Headers** section and ensure that the `ContentType` header is `application/json`.  Then under the **Body** section select **GraphQL** and enter this query:

```
{
    random(high:1000)
}
```

Each time you click the **Send** button in Postman you should see a new number between 1 and 1000 like this:

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

The eggql package automatically detects the problem and returns an error response like this:

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

### Making the Server Robust

GraphQL errors, like the wrong query name, are handled for you but what about other things?  What if the caller of the query made a mistake with the arguments?

```
{
    random(high:6, low:1)
}
```

With the Go code above this will cause `rand.Intn()` to panic and there will be no response.  To handle this a "resolver" function can return an `error`.  (A resolver function must return only a single value OR a single value plus an `error`.)

This is shown in the following code. Note that I have separated the type of the `Query` struct and declared the instance `q` separately for clarity as this example is growing.

```go
type Query struct {
	Random func(int, int) (int, error) `graphql:",params(low=1,high=6)"`
}

var q = Query{
	func(low, high int) (int, error) {
		if high < low {
			return 0, fmt.Errorf("random: high (%d) must not be less than low (%d)", high, low)
		}
		return low + rand.Intn(high+1-low), nil
	},
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}
```

Now the erroneous query will produce this result:

```json
{
    "errors": [
        {
            "message": "random: high (1) must not be less than low (6)"
        }
    ]
}
```

A critical part of any server in Go is using the `context.Context` type.  It allows all processing associated with a client request to be tidily terminated.  This is most commonly used for a timeout for a request in case anything is taking too long or has completely stalled.

Using **eggql** a resolver function can (optionally) take a 1st parameter of `context.Context`. You should almost always use a context in your resolver function unless you are sure it will always execute quickly and there is no chance of delay (eg the database could be offline or the network might be slow).  Moreover, since GraphQL queries can return lists and nested queries a single GraphQL request can cause a cascade of queries taking a long time even if each individual query does not.  The context approach can mitigate problems of overload due to poorly designed GraphQL queries or even a deliberate DOS attack.

To demonstrate I have added a `context.Context` parameter to the resolver function and added a loop with calls to `Sleep()` to simulate a process that may take a long time to run.  To enable the context I use the `http.TimeOutHandler()` specifying a time limit of 2 seconds.  If the resolver function is still running after 2 seconds the context `ctx` will be cancelled and the function can return as soon as it discovers that it's result is no longer required.

```go
type Query struct {
	Random func(context.Context, int, int) (int, error) `graphql:",params(low=1,high=6)"`
}

var q = Query{
	func(ctx context.Context, low, high int) (int, error) {
		if high < low {
			return 0, fmt.Errorf("random: high (%d) must not be less than low (%d)", high, low)
		}
		// simulate lengthy processing taking 10 seconds
		for i := 0; i < 10; i++ {
			if err := ctx.Err(); err != nil {
				return 0, err // abort lengthy processing if context is cancelled
			}
			log.Println(i)
			time.Sleep(time.Second)
		}
		return low + rand.Intn(high+1-low), nil
	},
}

func main() {
	http.Handle("/graphql", http.TimeoutHandler(eggql.MustRun(q), 2*time.Second, `{"errors":[{"message":"timeout"}]}`))
	http.ListenAndServe(":8080", nil)
}
```

Note that there are further ways to increase the robustness of your server, that I won't cover here, such as adding a ReadTimeout, graceful server shutdown, etc.  These are covered in Go HTTP tutorials found with a simple Google search.


