# EGGQL

## Why GraphQL?

You are probably already aware of the advantages of GraphQL.  In brief, it greatly facilitates **decoupling** of the backend (server) from clients.  The backend implements flexible building blocks (called **resolvers** in GraphQL) that allow queries to be constructed to (efficiently) access the data.

## Why Go?

Go simplifies creating any sort of software, but is particularly useful for writing server software due to its unequaled concurrency support.

Unfortunately, creating a GraphQL service can be cumbersome, even with Go, until now...

## Why EGGQL?

With EGGQL you don't need to worry about schemas, configuration files, transports, etc.  All you need is a struct (used to represent a GraphQL query) plus maybe a few field tags.

Some things you can do with just a few lines of code:

* serve static data (strings, numbers, etc) - see example below (**1. Hello**)
* serve existing maps, slices and arrays as GraphQL lists (**2. Friends**)
* auto-generate ID field for lists (map key, slice index) (**3. BetterFriends**)
* nested queries using nested structs (or pointers), slices or maps
* create dynamic data including query parameters (**4. Dynamic Data**)
* return meaningful errors, handle panics gracefully (**5. Handling Errors**)
* use context.Context for timeouts and cancelation (**6. Context Parameters**)

# Examples

## 1. Hello

This demonstrates a single GraphQL query of a static string.  Note that the query name "message" is automatically generated from the name of the `Message` field of the struct literal.

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

func main() {
    http.Handle("/graphql", eggql.MustRun(struct{ Message string }{"hello"}))
    http.ListenAndServe(":8080", nil)
}
```

### Testing with Curl

GraphQL requests are usually sent as an HTTP POST message.  You can easily query the server like this: 

```sh
$ curl -d '{"query": "{ message }"}' localhost:8080/graphql
```

### Testing with Postman

If you prefer you can use Postman for testing your service.  Just create a new **POST** request using an address of `http://localhost:8080/graphql`. Under the **Body** section select **GraphQL** and enter this query:

```graphql
{
    message
}
```

you should see a response like this:

```json
{
    "data": {
        "message": "hello"
    }
}
```

## 2. Friends

This example serves a slice as a GraphQL list.

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type Friend struct{ Name string }

// Create a query for a list of friends
var q = struct{ Friends []Friend }{
	Friends: []Friend{{"Alice"}, {"Bob"}, {"Carol"}},
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}
```

To test:

```sh
$ curl -d '{"query": "{ friends { name} }"}' localhost:8080/graphql
```

The query's list name "friends" is derived from the struct field name `Friends` (with first letter changed to lower-case). Similarly, the nested query name "name" comes from the `Name` field of the `Friend` struct.

The result is a list of friends with their names.

```json
{
  "data": {
    "friends": [
      {
        "name": "Alice"
      },
      {
        "name": "Bob"
      },
      {
        "name": "Carol"
      }
    ]
  }
}
```

## 3. Better Friends

Let's step it up a notch, with a map (instead of slice), using the friend's name as the map "key".  This has two queries one returning a list (of friends) and the retrieves individual friends given their ID (name in this case).

This demonstrates:

* using "egg" tags on struct fields
* overriding default names for the 2 queries (`friends` and `friend`)
* using a map for a GraphQL list
* using the "field_id" option - map key becomes field `name`
* using the same map to query individual elements in the list of friends
* using the "subscript" option to use `name` as the query parameter
* using `eggql.Date` type to display dates

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
	"time"
)

type Friend struct {
	Dob   eggql.Date
	Email string
}

var friends = map[string]Friend{
	"Alice": {Dob: Date(2006, 1, 2), Email: "alice@example.com"},
	"Bob":   {Dob: Date(1964, 2, 21)},
	"Carol": {Dob: Date(1996, 4, 16)},
}

// Create a query for a list of friends
var q = struct {
	List   map[string]Friend `egg:"friends,field_id=name"`
	Single map[string]Friend `egg:"friend,subscript=name"`
}{
	List:   friends,
	Single: friends,
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}

// Date creates an eggql.Date given (numeric) year, month and day
func Date(y, m, d int) eggql.Date {
	return eggql.Date(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC))
}
```

Here are some queries and the resulting JSON:

| query                                 | result (JSON)                                                                                                                                                                                |
|---------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| { friends { name } }                  | {"friends": [{"name": "Alice"}, {"name": "Bob"}, {"name": "Carol"}]}                                                                                                                         |
| { friend(name:"Alice") { email } }    | {"friend": { "email": "alice@example.com"}}                                                                                                                                                  |
| { friend(name:"Carol") { name dob } } | {"friend": { "name": "Carol", "dob": "1996-04-16"}}                                                                                                                                 |
| { friends { name dob } }              | {"friends": [{<br/>&nbsp; &nbsp;"name": "Alice", "dob": "2006-01-02"},<br/>&nbsp; &nbsp;{"name": "Bob", "dob":" 1964-02-21"},<br/>&nbsp; &nbsp;{"name": "Carol", "dob": "1996-04-16"}<br/>]} |

## 4. Dynamic Data

GraphQL queries usually don't return fixed data.  For example, many queries take arguments that affect what is returned.  For dynamic data you must use a Go `func` to generate the query results.

This example shows a query with two (optional) arguments that returns a random number.  The arguments specify the range of values, defaulting to a range of 1 to 6 (inclusive), possibly representing the sides of dice.

```go
package main

import (
	"github.com/andrewwphillips/eggql"
	"math/rand"
	"net/http"
)

type Query struct {
	Random func(int, int) int `egg:"random(low=1,high=6)"`
}

var q = Query{
	func(low, high int) int {
		return low + rand.Intn(high+1-low)
	},
}

func main() {
	rand.Seed(time.Now().UnixNano())
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}
```

Note that the Go `func` must have two parameters (`low` and `high`) since the resolver takes two arguments. (You can also have an optional initial `Context` function parameter that's not used as a query argument - see the **Context Parameters** example below.)

```sh
$ curl -d '{"query": "{ random(high:999) }"}' localhost:8080/graphql
```

gives a random response like this:

```json
{
    "data": {
        "random": 555
    }
}
```

### Query Errors

Try this to see what happens when you use the wrong query name:

```sh
$ curl -d '{"query": "{ rnd }"}' localhost:8080/graphql
```

The **eggql** package automatically detects the problem and returns an error response like this:

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

## 5. Handling Errors

GraphQL errors, like the wrong query name, are handled for you but what about errors that only your resolve can detect?  What if the caller of the query made a mistake with the arguments?

```graphql
{
    random(high:1, low:6)
}
```

With the Go code above this will cause `rand.Intn()` to panic (because it's given a -ve value) and the query will return this error:

```json
{
    "errors": [
        {
            "message": "internal error: invalid argument to Intn"
        }
    ]
}
```

This error message is not that useful to the client.  The resolver `func` could handle this better by returning an `error`.  (A resolver function must have either one or two return values, the 2nd one must be an `error` if provided.)

```go
type Query struct {
	Random func(int, int) (int, error) `egg:"(low=1,high=6)"`
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
	rand.Seed(time.Now().UnixNano())
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

## 6. Context Parameters

For resolvers that may take a long time to run and/or block on I/O you should also provide a **context** parameter.  In the code below I have added a `context.Context` as the 1st parameter of the `Random()` function and added a loop with a call to `Sleep()` to simulate a lengthy process.  An initial `context.Context` parameter is handled specially; it's not one of the resolver arguments.

To enable the context I use the `http.TimeOutHandler()` specifying a time limit of 2 seconds.  When the resolver function is still running after 2 seconds the context `ctx` will be cancelled and the function will return (with an error) as soon as it discovers that it's result is no longer required.

```go
type Query struct {
	Random func(context.Context, int, int) (int, error) `egg:"(low=1,high=6)"`
}

var q = Query{
	func(ctx context.Context, low, high int) (int, error) {
		if high < low {
			return 0, fmt.Errorf("random: high (%d) must not be less than low (%d)", high, low)
		}
		// simulate lengthy processing taking 10 seconds
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			if err := ctx.Err(); err != nil {
				return 0, err // abort lengthy processing if context is cancelled
			}
			log.Println(i)
		}
		return low + rand.Intn(high+1-low), nil
	},
}

func main() {
	rand.Seed(time.Now().UnixNano())
	http.Handle("/graphql", http.TimeoutHandler(eggql.MustRun(q), 2*time.Second, `{"errors":[{"message":"timeout"}]}`))
	http.ListenAndServe(":8080", nil)
}
```

Note that there are further ways to increase the robustness of your service, such as adding a ReadTimeout, graceful shutdown, etc.  These are easily incorporated into the above code.


# Details

To create a GraphQL service you must declare a struct, representing the root query object.  Each exported field (ie, having a capitalized name) of this struct represents a GraphQL query.  Each such field can be

- a scalar type (int, string, etc.) that represents a GraphQL scalar (Int!, String!, etc.)
- eggql.ID type that represents a GraphQL ID!, or *eggql.ID (ptr) to get a nullable ID
- for an enumeration: any integer type (int, int8, uint, etc.)
- a nested struct that represents a GraphQL nested query
- a slice/array/map that represents a GraphQL list of any of the above types
- a slice/array/map for which a "subscript" (single element) resolver is automatically generated
- a pointer to one of the above types, in which case the value is nullable
- a **function** that *returns* one of the above types.

A function is the most common resolver, except for simple, static data.  Using a function means the resolver result does not have to be calculated until required.  Also, one of the most powerful features of GraphQL is that resolvers can accept arguments to control their behaviour.  You have to use a function if the GraphQL resolver needs to take arguments.  See the above **Random Numbers** example which has a resolver that takes two arguments.

To use **eggql** you just need to call `eggql.MustRun()` passing an instance of the root query type.  You can also add mutations and subscriptions using the 2nd and 3rd parameters (see the [Star Wars Tutorial](https://github.com/AndrewWPhillips/eggql/blob/main/TUTORIAL.md) for an example.)  `MustRun()` returns an `http.Handler` which can be used like any other handler with the Go standard `net/http` package.

Note that the **Must** part of `MustRun()` indicates that no errors are returned - ie, it panics if anything goes wrong.  (You can instead get errors returned, as discussed below, which makes debugging easier.)  Importantly, it will only panic on problems detected at startup.  Once the service is up and running all errors are diagnosed and returned as part of the query response.  Even panics in your resolver functions are caught and returned as an "internal error:" followed by the panic message/data.

## Options

Several options are supported by adding them as parameters to the end of the `MustRun` function.  These are very easily added using [Functional Options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis).  For example, see the use of the **FuncCache** option in the next section.

### eggql.FuncCache(on bool)

This turns on caching of the results from func resolvers (non-func resolvers are not cached as they are already in memory).  To turn off caching for individual fields use the **no_cache** option of the egg: tag string.

Note that if a resolver takes arguments then different values are cached for each combination of used arguments.  As an example (from the Star Wars example) `Hero(NEWHOPE)` would cache _Luke Skywalker_, while `Hero(JEDI)` caches _R2D2_.

### eggql.NoIntrospection(on bool)

This disables all introspection queries.  This is sometimes done in production for security reasons.

### eggql.NoConcurrency(on bool)

By default, queries are executed concurrently.  This is always done when possible (subject to MAXPROCS), but, for example, a nested resolver cannot be executed until its parent resolver has completed.  Turning this option on means that resolvers (in a single query request) are executed sequentially.

Of course, requests from different clients (or even different requests from the same client) will still execute concurrently, so you cannot use this to avoid race conditions.  However, this option may be useful in testing to run a single query in order to find and track down race conditions in your resolvers.

### eggql.NilResolver(on bool)

By default, an error is returned for a resolver that is not implemented (nil func).  This option causes null to be returned for a resolver func that is nil.

This option is useful during development to stub resolver that have not yet been implemented.

### eggql.InitialTimeout(timeout time.Duration)

This sets the initial timeout for a subscription to be setup.  Technically, it is the time that the server waits for a "connection_init" message to be received after a websocket has been opened.  If the time is exceeded an error is generated and the websocket closed.

### eggql.PingFrequency(freq time.Duration)

For subscriptions, this is how often a "ping" message is sent on the websocket (or "ka" (keep alive) message, if the client is using the old GraphQL websocket protocol).

### eggql.PongTimeout(timeout time.Duration)

For subscriptions, this is how long to wait for a "pong" message after sending a "ping" to the client, before an error is generated and the websocket is closed.  (This only applies to the "new" GraphQL websocket protocol.)

## Caching

The result of func resolvers can be cached automatically using the `eggql.FuncCache` option.  By default, there is no caching.

As an example, add the `FuncCache()` call to the end of the `MustRun()` call in the `main()` function of the above random number server like this

```go
func main() {
	rand.Seed(time.Now().UnixNano())
	http.Handle("/graphql", eggql.MustRun(q, eggql.FuncCache(true)))
	http.ListenAndServe(":8080", nil)
}
```

Now whenever you invoke the random query you will get the same number.

```sh
$ curl -d '{"query": "{ random }"}' localhost:8080/graphql
```

Obviously, this is not desireable in this case (since the `Random` method is not a **pure** function).  You can turn caching off for specific resolvers, using the **no_cache** option of the egg: tag string like this:

```go
type Query struct {
	Random func(int, int) int `egg:"(low=1,high=6),no_cache"`
}
```

# Go GraphQL Packages

## Alternatives

There are other excellent, mature GraphQL packages for Go which may suit you better.

|                          Project                          | Developer(s)                                                        |
|:---------------------------------------------------------:|:--------------------------------------------------------------------|
|     [graphql](https://github.com/graphql-go/graphql)      | graphql-go (not to be confused with the project "graphql-go" below) |
| [graphql-go](https://github.com/graph-gophers/graphql-go) | graph-gophers                                                       |
|       [gqlgen](https://github.com/99designs/gqlgen)       | 99 Designs                                                          |
|      [thunder](https://github.com/samsarahq/thunder)      | Samsara Networks                                                    |

I particularly like **gqlgen** of **99 Designs** as it uses "go generate" to avoid the inefficiencies of reflection and the lack of type safety that is inevitable when using `interface{}` for polymorphism.

I recently discovered **thunder** which is based on the same premise as **eggql** (using reflection etc.), but it implements resolvers using Go interfaces (rather than closures).

The "pros" for **eggql** are, I believe, that it is simple to use (though I may be biased due to my familiarity with it :) and complete, and allows you to write robust GraphQL services due to support for `context.Context` parameters and `error` return values, handling panics, etc.  It also has special capabilities for working with in-memory slices and maps that the others don't.  It's probably fast enough unless you have a high-volume service (but there are areas where it can be improved).

The "cons" for **eggql** are that it *may not* be as performant as other packages [Ed: tests using **k6** seem to show that **eggql** is resolves simple queries as fast or faster than the other packages mentioned above]. such as **gqlgen** as it uses reflection and does not have performance options such as caching and data-loader (database support).  Also, resolver lookups currently use O(n) linear searches  [Ed: now fixed - using a map O(1)].  Custom scalars and a **date** type are not supported [Ed: they are now!].

## Performance Comparison

Out of interest, I recently did a performance comparison of the different packages using the **jMeter** and **k6**.  The results for **eggql** were surprisingly good, though take it with a grain of salt, until I have had independent confirmation.

See [COMPARISON.md](https://github.com/AndrewWPhillips/eggql/blob/main/COMPARISON.md)

## Vektah's gqlparser

I should also give a special shout-out to the Go **gqlparser** package upon which **eggql** is built.  This is an excellent library that I use to parse the GraphQL schemas that **eggql** generates and analyse and validate queries.  This package does all the hard work making implementing **eggql** a breeze.

* [gqlparser](https://github.com/vektah/gqlparser) by Vektah

# Highlights

Here are some important things not mentioned above.

## Tutorial

**Eggql** is a complete GraphQL implementation.  To see how easy it is to use there is a [Star Wars Tutorial](https://github.com/AndrewWPhillips/eggql/blob/main/TUTORIAL.md).  This explains how to implement a service for the **Star Wars** demo which almost all packages (in Go and other languages) have as an example.  It nicely shows how to use all standard features of GraphQL using **eggql**.  Unlike examples for other packages this one shows how to use **subscriptions**.

## Code-first GraphQL

My experience with many GraphQL packages/libaries is that they are confusing to set up (even the Go ones!), and it's hard to understand what is happening.  As a beginner I often mixed up the syntax of:

* queries
* query results (JSON)
* GraphQL schemas

Admittedly the first two are mainly of concern to clients of a GraphQL service, but you still need to use queries to test your service.  (To be fair the format of GraphQL queries, though not quite JSON, echoes the format of the results.)

My real issue is with *schemas*; they seemed unnecessary since Go data structures can serve the same purpose.  In other Go GraphQL packages the code will **panic if the schema and the Go structs are inconsistent** (though **gqlgen** mainly avoids this problem).  The DRY principle says to avoid having the same information in two places to (among other things) prevent problems with keeping it consistent.

My prime motivation, in creating **eggql** was to make it simpler to create a GraphQL service by bypassing the need to write a schema.  I have since discovered that others feel the same way leading to the "code-first" (schema-less) movement - for example see this recent post from the excellent LogRocket blog: [Code First vs Schema First GraphQL Development](https://blog.logrocket.com/code-first-vs-schema-first-development-graphql/):

## Reflection

Due to the way it works **eggql** makes extensive use of reflection, even though this may make the code a little slower.  [There are *many* things I like about Go but the main one is the emphasis on simplicity, even when it might affect performance a little, which is why Go code is usually 20% slower than equivalent C, Rust or Zig (but not 100-1000% slower like Python is :)].  I believe **eggql** is in the spirit of Go, by keeping things simple at the expense of a little performance.

## Lists and id Field

Many Go packages allow you to use an array as a GraphQL list.  With **eggql** you can also use a Go **map** as a GraphQL list field.  (Note that since the order of elements in a Go map is indeterminate the client should be aware that the order of the list is indeterminate and may even change for consecutive queries.)

**Eggql** can also generate an extra field for each object in an array/slice/map if you add the `id_field` option in the field's metadata tag.  For arrays and slices this represents the index of the element hence the generated field is of `Int!` type.  For a map, it is the map element's key type which must be an integer or string.

Here's a simple example server:
```go
type Query struct {
	Persons []struct {Name: string} `egg:",field_id=id"`  // list with fabricated "id" field
)

func main() {
	handler := eggql.MustRun(Query{ Persons:[]Person{{"Luke"}, {"Leia"}, {"Han"}}})
	http.Handle("/graphql", handler)
	http.ListenAndServe(":8080", nil)
}
```

If you run this query:

```graphql
{
    persons {
        id
        name
    }
}
```

you will see this result:

```json
{
    "data": {
        "persons": [
            {
                "id": 0,
                "name": "Luke"
            },
            {
                "id": 1,
                "name": "Leia"
            },
            {
                "id": 2,
                "name": "Han"
            }
        ]
    }
}
```

### Subscript Option

To make it even easier to allow your data to be accessed from GraphQL, **eggql** adds a "subscript" option (not to be confused with subscriptions).  This automatically generates GraphQL queries to access individual elements of slices, and arrays by their index, or maps by their key.

This is a unique capability of **eggql**.  Other GraphQL packages (at least all the ones I have tried in Go and other languages) allow you to get a list from an array or slice but have no such facility for maps and do not allow you to "subscript" into a list to retrieve individual elements.

As an example, say you have a map of information on record albums like this:

```Go
type Album struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Price  int    `json:"price"`
}

var albums = map[string]Album{
	"1": {Title: "Blue Train", Artist: "John Coltrane", Price: 56_99},
	"2": {Title: "Jeru", Artist: "Gerry Mulligan", Price: 17_99},
	"3": {Title: "Sarah Vaughan and Clifford Brown", Artist: "Sarah Vaughan", Price: 39_99},
}
```

To allow a GraphQL client to query on individual albums it's simply a matter of adding this line to your `Query struct`:

```Go
	A map[string]Album `egg:"album,subscript=id"`
```

The "subscript" option in the tag says to index into the list of albums using a query called "album" and an argument with a name of `id` and a type of `String!` (since the map's key is of type `string`).  The complete code for this example is in the "album" directory under example (see https://github.com/AndrewWPhillips/eggql/tree/main/example/album/main.go).

If you send this query:

```graphql
{
    album(id:"1") {
        title
        artist
    }
}
```

you will see this result:

```json
{
    "data": {
        "album": {
            "title": "Blue Train",
            "artist": "John Coltrane"
        }
    }
}
```

## Error-handling

There are two stages of error-handling when creating a GraphQL service:

1. coding/config errors that cause initial setup to fail, in which case `MustRun()` will panic
2. errors encountered when a query (or mutation) is running, whence an error message is returned to the client

### Viewing "startup" errors and the Schema

The 1st case is common when starting out -- you make lots of coding mistakes when creating structs, their fields, field tags (egg: key), enums, etc.  I'm not sure about you, but I always have to try to stay calm when I see "panic" on the screen or in the log.  Luckily, there is an alternative to using `MustRun()`.  Just call `eggql.New()`, then add things like enums etc. and call the `GetHandler()` method which returns an error instead of panicking if there is a problem.  This makes testing and debugging more pleasant.

Another advantage is that you can also call `GetSchema()` to view the GraphQL schema that **eggql*** has generated.

Here's a complete example. (Note: this example will likely change before the release of **eggql 1.0**.)

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"log"
	"net/http"
)

const value = 3.14

func main() {
	gql := eggql.New(struct {
		Len func(int) float64 `egg:"(unit:Unit=METER)"`    // *** 1 ***
	}{
		Len: func(unit int) float64 {
		    if unit == 1 {
			    return value * 3.28084 // return as feet
		    }
		    return value // return as meters
	    },
	})
	gql.SetEnums(map[string][]string{"Unit": []string{"METER", "FOOT"}})

	if schema, err := gql.GetSchema(); err != nil {        // *** 2 ***
		log.Fatalln(err)
	} else {
		log.Println(schema) // write the schema (text) to the log
	}
	if handler, err := gql.GetHandler(); err != nil {      // *** 3 ***
		log.Fatalln(err)
	} else {
		http.Handle("/graphql", handler)
		http.ListenAndServe(":8080", nil)                 // *** 4 ***
	}
}
```
As an explanation of the code - it provides a `len` query with an optional `unit` argument which can have values `METER` (default) or `FOOT` (see *** 1 *** ).  It also writes the generated schema to the log (*** 2 *** ).  Finally, it gets the handler (*** 3 *** ) and either logs the error or starts the server (*** 4 *** )

### Handling "runtime" errors

For the 2nd case of errors mentioned above (errors encountered during query execution), an error message is returned as part of the response to the client.

Note that even when there are errors GraphQL requests return an HTTP status of **OK** (200).  This includes errors that **eggql** detects while processing and validating the request, such as using an unknown query name.  It also includes errors returned from any resolver function, such as the "episode not found" error returned from the `Hero()` resolver function in the Star Wars Tutorial.  (GraphQL services do not usually generate HTTP status code like **Bad Request** (400), but this does not mean that a client should not be prepared to handle them.)

What about _bugs_ in the resolver functions?  If you detect a software defect in your code then you should return an error message beginning with "internal error:". An example is the "internal error: no character with ID" returned from the `Hero()` function in the Star Wars tutorial.

Also note that if your resolver function **panics** then the handler terminates, but the `panic` is recovered by **eggql** allowing the service to continue running and not affecting any concurrently running handlers.  The query result will contain an "internal error" and the text of the `panic`.  (Again HTTP status **Internal Server Error** (500) is *not* set.)  Of course, it's better to avoid panics, or gracefully return a useful error message, in your resolver functions.
