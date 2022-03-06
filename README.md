# EGGQL

The **eggql** package allows you to very easily create a GraphQL server using Go.

It currently supports queries, mutations, all GraphQL types including enums, interfaces, etc.  Queries (but not mutations) are processed concurrently.  It does not support subscriptions (yet).

For simplicity, you _don't_ need to create a GraphQL **schema**. You just declare Go structs with fields that act as the GraphQL **resolvers**.  For some things, like resolver arguments, you need to add tags (metadata attached to a field of a struct type), like the tags used to control JSON encoding/decoding.

## Getting Started

To create a GraphQL server you must declare a struct, representing the root query object.  Each exported field (ie, having a capitalized name) of this struct represents a GraphQL query.  Each such field can be

- a scalar type (int, string, etc) that represents a GraphQL scalar (Int!, String!, etc)
- an integer type (int, int8, uint, etc) that represents an enumeration
- a nested struct that represents a GraphQL nested query
- a slice or array that represents a GraphQL list of any of the above types
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
			Random func(int, int) int `graphql:",args(low=1,high=6)"`
		}{
			func(low, high int) int { return low + rand.Intn(high+1-low) },
		}))
	http.ListenAndServe(":8080", nil)
}
```

To test the server just send a query like the following to http://localhost:8080/graphql

```graphql
{
    random
}
```

Note that the query name `random` is derived from the struct's field name `Random`.  Only exported fields (those with a upper-case first letter) are used and the generated GraphQL name derived from it using a lower-case first letter.  You can also provide your own name using the graphql tag such as `graphql:"myRand,args(low=1,high=6)"`.

Also note the two resolver arguments (`low` and `high`) given in the graphql tag.  You must supply the `args` option of the tag if the resolver function takes arguments.  In this case there are two arguments so you must specify two names in the `args`.  (An exception is if the first function argument is a `context.Context` as we will see below.)

I usually test GraphQL servers using Postman (see below) but you can just use curl to post a GraphQL query like this:

```sh
$ curl -XPOST -d '{"query": "{ random }"}' localhost:8080/graphql
```

and you should get a response like this:

```json
{
    "data": {
        "random": 5
    }
}
```

### Testing with Postman

To use Postman for testing your server just create a new **POST** request using an address of `http://localhost:8080/graphql`. Under the address select the **Headers** section and ensure that the `ContentType` header is `application/json`.  Then under the **Body** section select **GraphQL** and enter this query:

```graphql
{
    random(high:1000)
}
```

Each time you click the **Send** button in Postman you should see a new number between 1 and 1000 like this:

```json
{
    "data": {
        "random": 467
    }
}
```

### Query Errors

Try this to see what happens when your use the wrong query name:

```sh
$ curl -XPOST -d '{"query": "{ rnd }"}' localhost:8080/graphql
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

### Making the Server Robust

GraphQL errors, like the wrong query name, are handled for you but what about other things?  What if the caller of the query made a mistake with the arguments?

```graphql
{
    random(high:6, low:1)
}
```

With the Go code above this will cause `rand.Intn()` to panic (because it's given a -ve value) and there will be no response.  To handle this a "resolver" function can return an `error`.  (A resolver function must return only a single value OR a single value plus an `error`.)

This is shown in the following code. Note that I have separated the type of the `Query` struct and declared the instance `q` separately for clarity as this example is growing in size.

```go
type Query struct {
	Random func(int, int) (int, error) `graphql:",args(low=1,high=6)"`
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

For resolvers that may take a long time to run or block on I/O you should also provide a **context** parameter.  In the code below I have added `context.Context` as the 1st parameter of the `Random()` function and added a loop with a call to `Sleep()` to simulate a lengthy process.  To enable the context I use the `http.TimeOutHandler()` specifying a time limit of 2 seconds.  If the resolver function is still running after 2 seconds the context `ctx` will be cancelled and the function will return (with an error) as soon as it discovers that it's result is no longer required.

```go
type Query struct {
	Random func(context.Context, int, int) (int, error) `graphql:",args(low=1,high=6)"`
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

Note that there are further ways to increase the robustness of your server, that I won't cover here, such as adding a ReadTimeout, graceful server shutdown, etc.  These are easily incorporated into the above code - look for Go HTTP tutorials using a Google search.

---

## Tutorial

GraphQL is all about types - scalar types (int, string, etc), object types which are composed of fields of other types (a bit like Go structs), lists (a bit like a Go slices) and more specialized types like interfaces and input types (which we will get to later).

Traditionally in GraphQL, you first create a schema which defines your types, but with **eggql** you just need to use Go structs and the schema is created for you.  We will see later how to view the GraphQL schema that is generated.  The first thing you need is a root query which is just an object type, but in this case it's fields define th queries that can be submitted to the GraphQL server.

Here I'll explain, in detail, how to declare Go types in order to implement a GraphQL server.  Since we're implementing the backend (GraphQL server) I'll focus on that, not on using it.  So it's mostly Go code with a few test queries.  However, I am using an example based on the official GraphQL tutorial (see https://graphql.org/learn/), so if you want to explorer the frontend side of things at the same time you can use the example queries from there.

First, we'll look at basic types (scalars, lists and nested objects).  Later we'll look at query arguments (including defaults and input types), enums, interfaces, mutations, etc.  We'll also look at the sorts of errors you can get and how to handle them.

Note: the final code for the tutorial is in example/starwars/main.go and the code is running in GCP.  So you can try any of the queries in Postman (or Curl etc) using the address https://aphillips801-eggql-sw.uc.r.appspot.com/graphql.  In fact you can try any of the queries from the offical GraphQL **Star Wars** tutorial at https://graphql.org/learn/.

### Basic Types

If _writing_ queries you also need to know about variables, fragments, directives, aliases, introspection, etc, but since these are handled automatically I won't cover them here.  There are plenty of tutorials that talk about how to use these things.

Here's a Go program for the first (`hero`) query of the GraphQL Star Wars tutorial (see https://graphql.org/learn/queries/).

```go
package main

import (
    "github.com/andrewwphillips/eggql"
    "net/http"
)

type (
	Query struct {
		Hero Character
	}
	Character struct {
		Name    string
		Friends []*Character
	}
)

func main() {
	http.Handle("/graphql", eggql.MustRun(
		Query{
			Hero: Character{
				Name: "R2-D2",
				Friends: []*Character{
					{Name: "Leia Organa"},
					{Name: "Luke Skywalker"},
				},
			},
		}))
	http.ListenAndServe(":8080", nil)
}
```

Here the type `Query` is the root query as it's the first (only) parameter passed to `MustRun()`.  Now you can send a `hero` query to the server which returns a `Character`.  The `Character` object can be queried for its name and for a list of friends.  Try this query:

```graphql
{
    hero {
        name
        friends {
            name
        }
    }
}
```

which will produce this:

```json
{
    "data": {
        "hero": {
            "name": "R2-D2",
            "friends": [
                {
                    "name": "Leia Organa"
                },
                {
                    "name": "Luke Skywalker"
                }
            ]
        }
    }
}
```

Note that you could recursively query the friends of the friends.  You can even query friends of friends of friends ... to any depth, but it is unwise to nest queries too deeply (and some servers limit nesting to 3 or 4 levels).  You can't see this with the above data as Luke and Leia do not have any friends yet.

The `Character` type is an object since it has fields (sub-queries) within it. `Query` is also an object but it is special being the **root query**.

The `Friends` field of `Character` defines a list, in this case implemented using a slice of pointers.  

The `Name` field has the GraphQL scalar type of `String!` because it uses the Go `string` type.  Similarly, any Go integer types create the GraphQL `Int!` type, Go bool => `Boolean!` and float32/float64 => `Float!`.  Note that none of these types are *nullable* by default, which is indicated by the GraphQL `!` suffix but can be made so by using pointers or the `nullable` tag.

Now we'll look at some more advanced types....

### Arguments

In GraphQL parlance the server code that "resolves" a query is called a resolver.  In the above example the "resolver" for the `hero` query was just a `Character` struct.  A more useful and more common thing is for a resolver to be a function.  For one, this allows resolvers to take arguments that permits much greater flexibility.

As an example we will change the `hero` resolver to be a function that takes a parameter specifying which episode we want the hero for.  So now instead of the `Hero` field simply being a `Character` object it is now a function that _returns_ a `Character`.

```Go
type Query struct {
	Hero func(episode int) Character `graphql:"hero,args(episode=2)"`
}
```
This also shows our first use of the **graphql tag** stored in the `Hero` field's *metadata*.  Metadata in Go can be attached to any field of a struct by adding a string after the field declaration.  (Note that these strings usually use back-quotes (`) rather than double-quotes (") so we don't have to _escape_ the double quotes within the string.)  The options in the graphql tag are comma-separated (using a similar format to json, xml, etc tags).

The first option in the graphql tag is the resolver name - in this case `hero`.  Although we don't really need to supply the name as it defaults to the field name (`Hero`) with the first letter converted to lower-case.

The 2nd option of the graphql tag (`args(episode=2)`) specifies the resolver arguments. The number of arguments (comma-separated) in the `args` option must match the number of function parameters (except that the function may also include an initial context.Context parameter as discussed below).  The names of the argument(s) must be given in the `args` (in this case there is just one called `episode`) and you can also give an optional default value (in this case `2`). [Technical note: we can't obtain the argument name from the function parameters as Go reflection only stores the types of function parameters not their names.]

Here is a complete program with the `Hero` resolver.  Note that I changed the `Hero()` function to return a pointer to `Character`; this allows us to return a null value when an invalid episode number has been provided as the argument. (I could have also changed the resolver function to return a 2nd `error` value as we will see later.)

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) *Character `graphql:",args(episode=2)"`
	}
	Character struct {
		Name    string
		Friends []*Character
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	characters = []Character{
		{Name: "Luke Skywalker"},
		{Name: "Leia Organa"},
		{Name: "Han Solo"},
		{Name: "R2-D2"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 0},
		{Name: "The Empire Strikes Back", HeroId: 0},
		{Name: "Return of the Jedi", HeroId: 3},
	}
)

func main() {
	// Set up friendships
	characters[0].Friends = []*Character{&characters[1], &characters[2], &characters[3]}
	characters[1].Friends = []*Character{&characters[0], &characters[2], &characters[3]}
	characters[2].Friends = []*Character{&characters[0], &characters[1]}
	characters[3].Friends = []*Character{&characters[0], &characters[1]}

	http.Handle("/graphql", eggql.MustRun(Query{Hero: func(episode int) *Character {
		if episode < 0 || episode >= len(episodes) {
			return nil
		}
		return &characters[episodes[episode].HeroId]
	}}))
	http.ListenAndServe(":8080", nil)
}
```

Now try this query:

```graphql
{
    hero(episode: 1) {
        name
    }
}
```

### Enums

In the above code we used an integer to identify episodes.  For example, the `Hero` function's parameter is `episode int`.  For this type of data GraphQL provides enums, which are essentially an integer restricted to as set of named values.  If you are familiar with GraphQL schemas we can define a new `Episode` enum type with 3 values type like this:

```graphqls
enum Episode {
  NEWHOPE
  EMPIRE
  JEDI
}
```
(Don't worry if you are not familiar with schemas as **eggql** will generate the schema automatically.)

The three allowed values for an `Episode` (NEWHOPE, EMPIRE, and JEDI) are internally represented by the integers 0, 1, and 2.  Because Go does not have a native enum type you just use an integer type, but you also need to tell **eggql** about the corresponding names of the enum values using a slice of strings.  To do this you pass a map of string slices as the 1st (optional) parameter to `MustRun()` where the map key is the enum name.  For example, here is the map for two enum types `Episode` and `Unit`.

```Go
var gqlEnums = map[string][]string{
	"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
	"Unit":    {"METER", "FOOT"},
}
```

It's simple to change the `Hero` resolver to use this `Episode` enum as its argument.

```Go
	Hero func(episode int) *Character `graphql:",args(episode:Episode=JEDI)"`
```

If you look closely at the above `args` option you can see that the `episode` argument now has a type name after the colon (:) which is the enum name.  The default value is changed from the integer literal `2` to the enum value `JEDI`.

Of course, a resolver can also _return_ an enum, or a list of enums as we will show here.  We will add a new `appearsIn` field to the `Character` struct which contains a list of episodes that the character has appeared in.

```Go
	Appears []int `graphql:"appearsIn:[Episode]"`
```

Here the first tag option (`appearsIn:[Episode]`) says that the GraphQL name of the field is `appearsIn` and the type is a list of `Episode`.

Here's the final program with the above changes.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) *Character `graphql:",args(episode:Episode=JEDI)"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `graphql:"appearsIn:[Episode]"`
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"},
	}
	characters = []Character{
		{Name: "Luke Skywalker"},
		{Name: "Leia Organa"},
		{Name: "Han Solo"},
		{Name: "R2-D2"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 0},
		{Name: "The Empire Strikes Back", HeroId: 0},
		{Name: "Return of the Jedi", HeroId: 3},
	}
)

func main() {
	// Set up friendships
	characters[0].Friends = []*Character{&characters[1], &characters[2], &characters[3]}
	characters[1].Friends = []*Character{&characters[0], &characters[2], &characters[3]}
	characters[2].Friends = []*Character{&characters[0], &characters[1]}
	characters[3].Friends = []*Character{&characters[0], &characters[1]}

	// Set up appearances
	characters[0].Appears = []int{0, 1, 2}
	characters[1].Appears = []int{0, 1, 2}
	characters[2].Appears = []int{0, 1, 2}
	characters[3].Appears = []int{0, 1, 2}

	http.Handle("/graphql", eggql.MustRun(gqlEnums, Query{Hero: func(episode int) *Character {
		if episode < 0 || episode >= len(episodes) {
			return nil
		}
		return &characters[episodes[episode].HeroId]
	}}))
	http.ListenAndServe(":8080", nil)
}
```

If you run this query:

```graphql
{
    hero(episode: EMPIRE) {
        name
        appearsIn
    }
}
```

You should see this result:

```json
{
    "data": {
        "hero": {
            "name": "Luke Skywalker",
            "appearsIn": [
                "NEWHOPE",
                "EMPIRE",
                "JEDI"
            ]
        }
    }
}
```

### Interfaces

Interfaces (and unions) are an advanced, but sometimes useful, feature of GraphQL.  Interfaces are similar to interfaces in the type system of Go, so you may be surprised that **eggql** does not use Go interfaces to implement GraphQL interfaces.  Instead it uses struct embedding.

To demonstrate interfaces we are going to change the Star Wars example so that the `Character` type is an interface.  But first we introduce two new types `Human` and `Droid` for GraphQL object types that **implement** the `Character` interface.

```Go
type (
	Human struct {
		Character
		Height float64 // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
)
```

The above Go code creates two GraphQL types `Human` and `Droid` which implement the `Character` interface because the Go struct embeds the `Character` struct.  (They also have their own type specific fields.)

No changes are required to the earlier `Character` struct but it now is used as a GraphQL `interface` due solely to the fact that it has been embedded in another struct (or two in this case).

If you have a Character struct (or pointer to one) there is no way in Go to get the struct that embeds it or to even determine that it is embedded in another struct.  So to return a `Character` (which is either a `Human` or a `Droid` underneath) we return a Human or Droid as a Go `interface{}` and use the graphql tag to indicate that the GraphQl type is a `Character` interface.

```Go
	Hero func(episode int) interface{} `graphql:"hero:Character,args(episode:Episode=JEDI)"`
```

Here the first option in the graphql tag (`hero:Character`) says that the field is called `hero` and its type is `Character`.  Of course. you also need to change the implementation of the Hero() function so that it returns a `Human` or a `Droid` (as an `interface{}`).

This change to the return value causes one further complication that given the `Query` type passed to `MustRun()` there is no way for **eggql** to discover (by reflection) the `Character` type or even the new `Human` and `Droid` types.  The solution is to add a dummy field with a "blank" name of underscore (_).  See below.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `graphql:"hero:Character,args(episode:Episode=JEDI)"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `graphql:"appearsIn:[Episode]"`
	}
	Human struct {
		Character
		Height float64 // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"},
	}
	humans = []Human{
		{Character{Name: "Luke Skywalker"}, 1.67},
		{Character{Name: "Leia Organa"}, 1.65},
		{Character{Name: "Han Solo"}, 1.85},
		{Character{Name: "Chewbacca"}, 2.3},
	}
	droids = []Droid{
		{Character{Name: "R2-D2"}, "Astromech"},
		{Character{Name: "C-3PO"}, "Protocol"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2000},
	}
)

func main() {
	// Set up friendships
	luke := &humans[0].Character
	leia := &humans[1].Character
	solo := &humans[2].Character
	chew := &humans[3].Character
	r2d2 := &droids[0].Character
	c3po := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, r2d2}
	humans[1].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{c3po, luke, leia}
	droids[1].Friends = []*Character{r2d2, leia}

	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}

	http.Handle("/graphql", eggql.MustRun(gqlEnums, Query{Hero: func(episode int) interface{} {
		if episode < 0 || episode >= len(episodes) {
			return nil
		}
		ID := episodes[episode].HeroId
		if ID >= 2000 {
			// droids have IDs starting at 2000
			ID -= 2000
			if ID >= len(droids) {
				return nil
			}
			return droids[ID]
		}
		// humans have IDs starting at 1000
		ID -= 1000
		if ID < 0 || ID >= len(humans) {
			return nil
		}
		return humans[ID]
	}}))
	http.ListenAndServe(":8080", nil)
}
```

You can check that this works using a GraphQL query with **inline fragments**.  (See [Inline Fragments](https://graphql.org/learn/queries/#inline-fragments) for details.)

```graphql
{
  hero {
    name
    ... on Droid {
      primaryFunction
    }
    ... on Human {
      height
    }
  }
}
```

which will produce JSON output like this:

```json
{
    "data": {
        "hero": {
            "name": "R2-D2",
            "primaryFunction": "Astromech"
        }
    }
}
```

Since R2-D2 is a droid you get the `primaryFunction` field.  Now use a differeny episode as the query parameter, as below.

Note that rather than changing the query in this way it is good practice to "parameterize" any parts that might change using variables - see [GraphQL Variables](https://graphql.org/learn/queries/#variables).  But that's outside the scope of this tutorial.

```graphql
{
  hero(episode: NEWHOPE) {
    name
    ... on Droid {
      primaryFunction
    }
    ... on Human {
      height
    }
  }
}
```

which returns Luke's modest height:

```json
{
    "data": {
        "hero": {
            "name": "Luke Skywalker",
            "height": 1.67
        }
    }
}
```

### Mutations and Input Types

GraphQL is mainly used for receiving information from the backend (server) using queries, but it's sometimes required for a frontend (client) to _send_ information to the backend.  This is what Mutations are for.  Mutations are syntactically identical to queries.  There is a root mutation (by default called `Mutation`) in the same way there is a root query (by default called `Query`). Just as the root query is passed as the 1st parameter to `MustRun()` (2nd parameter if you have enums) then the root mutation is passed as the next (2nd or 3rd parameter).

Are mutations necessary? A _query_ could in fact modify data on the backend but that is a bad idea for two reasons:

1. It's confusing to clients (and to backend code maintainers).
2. Mutations are guaranteed to be executed in sequence (whereas parts of a query may resolve in parallel).  So the order that changes would be made in queries is undefined and the results would be unpredictable. 

To demonstrate, we will add a mutation that allows clients to submit movie ratings and reviews.  Here are the relevant types:

```Go
type (
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
	}
	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `graphql:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)
```
We've already seen the `EpisodeDetails` struct, but it now has two new fields (`Stars` and `Commentary`) which are used to save the submitted values from the client.  There is also a root mutation containing the `CreateReview` mutation.  This mutation takes two arguments, an `Episode` (enum) and a `ReviewInput` and returns the `EpisodeDetails` as confirmation of the change.

#### Input Types

A new thing here is a struct (`ReviewInput`) used as an argument.  This creates a GraphQL **input** type.  An input type is similar to a GraphQL object type except that it can only be used as an argument to a mutation or query.  Unlike an object (or interface) type the fields of an input type cannot have arguments.  Also, if you try to use the same Go struct as an input type _and_ an object (or interface) type then **eggql** will return an error.  TODO: include error message

Here is the complete program with the `CreateReview` mutation.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `graphql:"hero:Character,args(episode:Episode=JEDI)"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `graphql:"appearsIn:[Episode]"`
	}
	Human struct {
		Character
		Height float64 // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `graphql:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"},
	}
	humans = []Human{
		{Character{Name: "Luke Skywalker"}, 1.67},
		{Character{Name: "Leia Organa"}, 1.65},
		{Character{Name: "Han Solo"}, 1.85},
		{Character{Name: "Chewbacca"}, 2.3},
	}
	droids = []Droid{
		{Character{Name: "R2-D2"}, "Astromech"},
		{Character{Name: "C-3PO"}, "Protocol"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2000},
	}
)

func main() {
	// Set up friendships
	luke := &humans[0].Character
	leia := &humans[1].Character
	solo := &humans[2].Character
	chew := &humans[3].Character
	r2d2 := &droids[0].Character
	c3po := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, r2d2}
	humans[1].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{c3po, luke, leia}
	droids[1].Friends = []*Character{r2d2, leia}

	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}

	http.Handle("/graphql", eggql.MustRun(gqlEnums,
		Query{
			Hero: func(episode int) interface{} {
				if episode < 0 || episode >= len(episodes) {
					return nil
				}
				ID := episodes[episode].HeroId
				if ID >= 2000 {
					// droids have IDs starting at 2000
					ID -= 2000
					if ID >= len(droids) {
						return nil
					}
					return droids[ID]
				}
				// humans have IDs starting at 1000
				ID -= 1000
				if ID < 0 || ID >= len(humans) {
					return nil
				}
				return humans[ID]
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) *EpisodeDetails {
				if episode < 0 || episode >= len(episodes) {
					return nil
				}
				episodes[episode].Stars = review.Stars
				episodes[episode].Commentary = review.Commentary
				return &episodes[episode]
			},
		},
	))
	http.ListenAndServe(":8080", nil)
}
```

To run the `CreateReview` mutation send this "query" to the server:

```graphql
mutation {
  createReview(episode: EMPIRE, review: {stars: 5, commentary: "one of the greatest science fiction movies"}) {
    name
    stars
    commentary
  }
}
```

You should get a response that confirms that the `EpisodeDetails` was updated

```json
{
    "data": {
        "createReview": {
            "name": "The Empire Strikes Back",
            "stars": 5,
            "commentary": "one of the greatest science fiction movies"
        }
    }
}
```

### Using Go Methods as Resolvers

One of the great things about **eggql** is that you may be able to quickly turn existing software into a GraphQL server.  This is because you can often use existing structs to generate GraphQL types, with little or no changes to the code.  As long as the field name is capitalized then the field will automatically resolve to its value.  A good example of this is the `height` field of the `Human` struct we have already seen.

One complication with resolver functions is: How do they access the data of their parent?  The first thing is to remember that the Go `func` type is more than a function pointer but a **closure**.  (I guess I should really call them resolver "closures" not resolver functions.) So far we have only assigned a function to a resolver `func` but (as it's a closure) it can also be assigned an instance **method**, in which case it retains a pointer to the instance.

(I tend to think of a Go `func` variable like a C function pointer, and when you assigned a Go _function_, or nil, to it, it is essentially no more than a function pointer.  But it can be _more_ since a closure retains two things: a function pointer and data pointer.)

Imagine we need to change the `height` resolver so that it takes an argument specifying the unit (foot or meter) in which we want the value returned.  To use an argument the `Height float64` field must be converted to a closure that returns a `float64`.

```Go
	Human struct {
		Character
		Height       func(int) float64 `graphql:",args(unit:Unit=METER)"`
		height       float64            // meters
	}
```

Now the `Height` resolver is a `func` taking a "unit" argument.  The `unit` argument is of type `Unit` - an enum we introduced earlier but did not use.  The original `Height` field is retained but with an initial lower-case 'h' so that it is *not* seen as a GraphQL field.

The problem now is how does the `Height` closure access the `height` field?  To do this we introduce a method on the `Human` type which I called `getHeight()`.  This method has the same signature as the `Height` closure (`func(int) float64`).

```Go
func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case FOOT:
		return h.height * 3.28084
	}
	return h.height
}
```

This method is assigned to the `Height` field.  Since we have a slice of `Human`s we have to do it for each element of the slice.

```Go
	for i := range humans {
		humans[i].Height = (&humans[i]).getHeight
	}
```

With this change, if you add a `unit:FOOT` argument to the `height` sub-query:

```graphql
{
    hero(episode:NEWHOPE) {
        name
        ... on Human {
            height(unit:FOOT)
        }
    }
}
```

You will get Luke's height in feet instead of meters:

```json
{
    "data": {
        "hero": {
            "name": "Luke Skywalker",
            "height": 5.4790028
        }
    }
}
```

### Error Handling

#### Resolver errors

Resolver `func`s return a single value, but they can optionally return an `error` which will be reported in the "errors" section of the query result.  For example, in the `Hero` function we used above, when there is an error we return `nil` instead of a pointer to a `Character`, which results in a `NULL` `Character` being seen in the GraphQL query results.

There are two error conditions in the `Hero` resolver.

1. the client provides an invalid episode as the query parameter
2. the hero ID stored in the `EpisodeDetails` does not refer to a real character

So to distinguish between these errors we add a 2nd (`error`) return value to the `Hero` function (see the complete program below).  Now if there is an error the query will return an error message instead of just a `NULL` character.

#### Contexts

A critical part of any server in Go is using the `context.Context` type.  It allows _all_ processing associated with a client request to be expediently and tidily terminated.  This is most commonly used in web servers for a timeout in case anything is taking too long or has completely stalled.

Using **eggql** a resolver function can (optionally) take a 1st parameter of `context.Context`.  You would almost certainly use a context if the resolver code read from or wrote to a Go `chan`, or made a library or system call that could block on disk or network I/O such as a database query.  A less common scenario is a compute intensive resolver in which case you can check if the context has been cancelled regularly, such as in an inner loop.

Our Star Wars example works with in-memory data structures so the resolver functions do _not_ need context parameters.  (See the **Getting Started** example in the README where a `context` parameter is added to the `random` query.)  Even so, since GraphQL queries can return lists and nested queries, a single GraphQL request can cause a cascade of queries taking a long time even if each individual query does not - eg. if you deeply nested a friends query like this: 

```graphql
{
    hero {
        name
        friends {
            name
`            friends {
               ...
            }
        }
    }
}
```

Fortunately, **eggql** itself will automatically shutdown query processing if the `context` is cancelled.  If you test this with a deeply nested query (and perhaps reduce the timeout in the `TimeoutHandler` below), you will see a message like this, even without `Hero` using a `Context` parameter:

```json
{
  "errors":[
    {
      "message":"timeout"
    }
  ]
}
```

Note that Go HTTP handlers do **not** have timeouts by default, so the GraphQL handler is wrapped in a timeout handler (see the call to `http.TimeoutHandler()`) which creates a context that expires after 5 seconds.  Using a `Context` like this can mitigate problems such as poorly designed client GraphQL queries, server overload or even a DOS attack.

```Go
package main

import (
	"fmt"
	"github.com/andrewwphillips/eggql"
	"net/http"
	"time"
)

type (
	Query struct {
		Hero func(episode int) (interface{}, error) `graphql:"hero:Character,args(episode:Episode=JEDI)"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `graphql:"appearsIn:[Episode]"`
	}
	Human struct {
		Character
		Height func(int) float64 `graphql:",args(unit:Unit=METER)"`
		height float64           // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `graphql:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"},
	}
)

var (
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2001},
	}
)

func init() {
	// Set up friendships
	luke := &humans[0].Character
	leia := &humans[1].Character
	solo := &humans[2].Character
	chew := &humans[3].Character
	c3po := &droids[0].Character
	r2d2 := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, r2d2}
	humans[1].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{r2d2, leia}
	droids[1].Friends = []*Character{c3po, luke, leia}

	// Set up human Height closure
	for i := range humans {
		humans[i].Height = (&humans[i]).getHeight
	}
	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}
}

func main() {
	handler := eggql.MustRun(
		gqlEnums,
		Query{
			Hero: func(episode int) (interface{}, error) {
				if episode < 0 || episode >= len(episodes) {
					return nil, fmt.Errorf("episode %d not found", episode)
				}
				ID := episodes[episode].HeroId
				if ID >= 2000 {
					// droids have IDs starting at 2000
					ID -= 2000
					if ID < len(droids) {
						return droids[ID], nil
					}
				} else {
					// humans have IDs starting at 1000
					ID -= 1000
					if ID >= 0 && ID < len(humans) {
						return humans[ID], nil
					}
				}
				return nil, fmt.Errorf("internal error: no character with ID %d in episode %d", ID, episode)
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) *EpisodeDetails {
				if episode < 0 || episode >= len(episodes) {
					return nil
				}
				episodes[episode].Stars = review.Stars
				episodes[episode].Commentary = review.Commentary
				return &episodes[episode]
			},
		},
	)
	handler = http.TimeoutHandler(handler, 5*time.Second, `{"errors":[{"message":"timeout"}]}`)
	http.Handle("/graphql", handler)
	http.ListenAndServe(":8080", nil)
}

// getHeight returns the height of a human
// Parameters
//  h (receiver) is a pointer to the Human
//  unit is the unit for the return value
func (h *Human) getHeight(unit int) float64 {
	if unit == 1 {
		return h.height * 3.28084
	}
	return h.height
}
```

# Viewing Errors and the Schema

There are two classes of errors you may need to deal with:

1. coding errors that cause initial setup to fail, in which case `MustRun()` will panic
2. errors in the query that will result in an error response being returned to the client

It is common to initially make lots of coding mistakes when creating structs, their fields, their metadata (graphql tag), enums, etc.  Instaed of getting a panic when you using `MustRun()` you can get the error returned by using object returned from `eggql.New()`, then adding the query and mutation structs as well as enums, then call the `GetHandler()` method.  You can also call `GetSchema()` to view the GraphQL schema that **eggql*** has generated.  Here's an example:

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"log"
	"net/http"
)

const value = 3.14

func main() {
	gql := eggql.New()
	gql.SetQuery(struct {
		Len func(int) float64 `graphql:",args(unit:Unit=METER)"`
	}{
		Len: func(unit int) float64 {
		    if unit == 1 {
			    return value * 3.28084 // return as feet
		    }
		    return value // return as meters
	    },
	})
	gql.SetEnums(map[string][]string{"Unit": []string{"METER", "FOOT"}})

	if schema, err := gql.GetSchema(); err != nil {
		log.Fatalln(err)
	} else {
		log.Println(schema) // write the schema (text) to the log
	}
	if handler, err := gql.GetHandler(); err != nil {
		log.Fatalln(err)
	} else {
		http.Handle("/graphql", handler)
		http.ListenAndServe(":8080", nil)
	}
}
```

Once the GraphQL server is running - ie. the handler is processing then any errors encountered will return an error response to the client.  All requests are returned with an HTTP status of **OK** (200).  There is no way to return a different HTTP status like **Bad Request** (400) for GraphQL errors.  This includes errors that **eggql** detects while processing and validating the request, such as using an unknown query name.  It also includes errors returned from any resolver function, such as the "episode not found" error returned from the `Hero()` resolver function above.  (This does not mean that a client of the GraphQL server should not be prepared to handle an HTTP status error.)

What about _bugs_ in the resolver functions?  If you detect a software defect in your code then you should return an error message beginning with "internal error:".  There is no way to return HTTP status **Internal Server Error** (500).  An example is the "internal error: no character with ID" returned from the `Hero()` function above.

Also note that if your resolver function **panics** then (fortunately) the program will not terminate.  The Go http package will "catch" any panic in a handler and terminate the request, but continue to allow further requests to be processed.  Of course, you should not allow your resolver functions to panic (as the client will get no response) but at least return some sort of internal error.

### Conclusion

I trust this tutorial has helped you to see how easy it is to create a simple GraphQL server using **eggql**.  Unlike most backend soltuions you are not required to create, or even understand GraphQL schemas.  (Under the hood, a schema is generated for you which you can view if you need to.)  Unlike other Go packages this avoids getting lot's of run-time panics when your schema does not match your data types.

However, **eggql** may not be the best solution for you if you want something comprehensive or more efficient.  It does not have any support for databases, such as a dataloader since I wrote it to work with in-memory data.  It may also be too slow for heavy load as it uses reflection to run resolvers.  Here are some other Go GraphQl packages that I have tried and found to work very well.

[graphql](https://github.com/graphql-go/graphql) by "graphql-go" (not to be confused with the project "graphql-go" below)
[graphql-go](https://github.com/graph-gophers/graphql-go) by graph-gophers
[gqlgen](https://github.com/99designs/gqlgen)  by 99 Designs

I particularly like **gqlgen** of **99 Designs** as it uses "go generate" to avoid the inefficiencies of reflection and the lack of type safety due to passing around `interface{}` types. 

The "pros" for **eggql** are, I believe, that  it is simple to use (though I may be biased due to my familiarity with it) and complete (except for subscriptions that currently have high priority), and allows you to write robust GraphQL servers due to support for `context.Context` parameters and `error` return values.  I have also found it surprisingly performant.

The latest (and possibly improved) version of the code of the Star Wars example is available on GitHub at [Star Wars example](https://github.com/AndrewWPhillips/eggql/tree/main/example/starwars).  This includes more data (characters etc) and more queries and fields but is still less than 200 lines of code.
