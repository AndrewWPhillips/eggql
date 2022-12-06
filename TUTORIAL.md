## Star Wars Tutorial

This tutorial will show you how to implement a GraphQL service using Go and **eggql**.  (See the corresponding GraphQL [Star Wars Frontend Tutorial](https://graphql.org/learn/) for how you can query this service.)  It shows how to implement every feature of a GraphQL service except for subscriptions.  (Subscriptions are also not covered in the **Star Wars** frontend tutorial.)

Remember, this tutorial is about implementing the backend (a GraphQL service), so I'll focus on creating the service, not on using it.  It's mainly Go code with a few test queries.  We **won't** look at aliases, fragments, inline fragments, variables, directives, schemas and introspection, but rest assured they work.  These things are all covered in the GraphQL **Star Wars** frontend tutorial if you want to try the queries from that.

You don't need to know a lot about Go to create useful services. Although internally **eggql** uses reflection, that is hidden except that you need to understand how to use field tags (metadata), but you've probably already encountered this in encoding/decoding of JSON (or gob/xml/yaml/etc).  You also don't need to understand GraphQL schema syntax as **eggql** creates the schema based on your struct fields and their metadata.

You should probably follow this tutorial sequentially as each section builds on the previous, but here are some links if you need to quickly find specific information:

1. [Basic Types](#basic-types) - objects (including nested objects), lists and scalars (String, Int, etc)
2. [Resolvers and Arguments](#resolver-functions-and-arguments) - a resolver function can have argument(s) to refine the data returned
3. [Enums](#enums) - GraphQL has enum types but Go doesn't, so they need to be handled specially
4. [Mutations](#mutations-and-input-types) - Queries retrieve data, Mutations modify the data
5. [Input Types](input-types) - mutations (and queries) sometimes need complex arguments
6. [Interfaces](#interfaces) - similar objects can implement an interface to indicate a common behaviour (polymorphism)
7. [Unions](#unions) - like interfaces except the types in the union have no fields in common
8. [Descriptions](#descriptions) - fields, arguments, etc can have a description to be used by query designers
9. [Viewing Descriptions using Introspection](#using-introspection-to-obtain-descriptions)
10. [Errors](#resolver-errors) - how to handle errors
11. [Contexts](#contexts) - contexts are used to cancel GraphQL queries - e.g. for a timeout if they take too long to run
12. [Methods as Resolvers](#using-go-methods-as-resolvers) - how a resolver function can easily access data of its parent object
13. [Custom Scalars](#custom-scalars) - you can add new types to be used in your queries called custom scalars
14. [Subscriptions](#subscriptions) - Subscriptions are a powerful way to provide a continuous stream of data

Note: the final code for this tutorial is in the git repo (https://github.com/AndrewWPhillips/eggql/tree/main/example/starwars).  It's less than 250 lines of code (and most of that is just data field initialization since all the data is stored in memory).  This is much smaller (and I think much simpler) than the equivalent Star Wars example using other GraphQL packages (for Go and even other languages).  The complete version is also running *right now* in GCP (Google Cloud Platform), so you can try any Star Wars queries using GraphIQL, Postman, Curl etc., using the address https://aphillips801-eggql-sw.uc.r.appspot.com/graphql.

### Basic Types

GraphQL is all about types - *scalar* types (int, string, etc), *object* types composed of fields of other types (a bit like Go structs), lists (a bit like a Go slice or array) and more specialized types like interfaces, unions and input types (which we will get to later).

Traditionally when building a GraphQL service, you first create a schema which defines your types, but with **eggql** you just use Go structs; *the schema is created for you*.  (To see the generated schema refer to [Viewing the Schema](https://github.com/AndrewWPhillips/eggql#viewing-errors-and-the-schema) in the README.)  The first thing you need is a **root query** which is just like any other GraphQL object type.  The **fields** of the root query define the queries that can be submitted to the GraphQL server.

First we'll add queries returning basic types (scalars, lists and nested objects).  Then we'll look at how to implement query arguments, mutations, and more advanced types.  We'll also look at the sorts of errors you can get and how to handle them.

To start create a new Go project with one Go file called **main.go** and add the code below, then build and run it.  You may need to run `go mod tidy` so that the **eggql** package is downloaded and added to your **go.mod** file.

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
This program handles the first (`hero`) query of the GraphQL Star Wars tutorial (see https://graphql.org/learn/queries/).

To explain the code: the type `Query` is the root query as it's used as the first (only) parameter for `MustRun()`.  With this service running you can post a `hero` query to the server which returns a `Character`.  The `Character` object can be queried for its name and for a list of friends.

To test it try the query below using the address http://localhost:8080/graphql (this address comes from the parameters to `Handle` and `ListenAndServe` in the code above).  Eg using Curl:

```sh
$ curl -XPOST -d '{"query": "{hero {name friends {name}}}"}' localhost:8080/graphql
```

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

Note that you could recursively query the friends of the friends.  You can even query friends of friends of friends ... to any depth, but it is unwise to nest queries too deeply (and some servers limit nesting to 3 or 4 levels).  Unfortunately, you can't test this with the above data as Luke and Leia sadly do not have any friends (yet).

The `Character` type is used as a GraphQL object since it has fields (sub-queries) within it. `Query` is an object, but it is special, being the **root query**.

The `Friends` field provides a list of `Character`, in this case implemented using a slice of pointers.

The `Name` field has the GraphQL scalar type of `String!` because it uses the Go `string` type.  Similarly, any Go integer types create the GraphQL `Int!` type, Go bool => `Boolean!` and float32/float64 => `Float!`.  Note that these types are followed by an exclamation mark (!) which indicates they are not nullable.  You can get the nullable version by using pointers or adding `nullable` to the field tag (as explained later).

Now we'll look at some more advanced types....

### Resolver Functions and Arguments

In GraphQL the server code that processes a query is called a **resolver**.  In the above example the "resolver" for the `hero` query was just a `Character` struct.  A more useful and more common thing is for a resolver to be a function (technically a Go **closure**).  Using functions for resolvers has advantages, as it allows:

* efficiency, since the function is not executed unless or until its specific data is required
* use of values which are initially unknown or dynamic (e.g. random numbers as in the README example)
* recursive queries (e.g. friends of friends of ...), which can't be pre-computed (without infinite recursion :)
* and function resolvers can take arguments that can modify or refine the results returned

It's the last advantage that we look at here - resolver arguments.  As an example we can change the `hero` resolver to be a function that takes a parameter specifying which episode we want the hero for.  So instead of the `Hero` field just being a `Character` object it is now a function that _returns_ a `Character`.

```Go
type Query struct {
	Hero func(episode int) Character `egg:"hero(episode=2)"`
}
```
This also shows our first use of the **egg: key** taken from the `Hero` field's **tag string**.  This sort of metadata in Go can be attached to any field of a struct by adding a string after the field declaration.

You can have several (comma-separated) options in the tag string but the first one is the most important as it allows you to specify the resolver name, its type, and it's arguments. Each argument has a name and optionally a type and default value.

The first option in the `egg:` metadata is the resolver name - in this case `hero`.  We don't really need to supply the name, in this case, as it defaults to the field name (`Hero`) with the first letter converted to lower-case.

In brackets after the name (e.g.. `(episode=2)`) are the resolver argument(s). The number of arguments (comma-separated) in the `args` option must match the number of function parameters (except that the function may also include an initial `context.Context` parameter as discussed later).  The names of the argument(s) must be given (in this case there is just one called `episode`).  You can give a GraphQL type but here it is deduced to be `Int!` from the function argument type.  You can also give an optional default value (in this case `2`). [Technical note: you always need to supply a name since we can't obtain the argument name from the `Hero` function parameter name as Go reflection only provides the types of function parameters not their names.]

Here is a complete program with the `func` resolver. I changed the `Hero()` function to return a pointer to `Character`; this allows us to return a null value when an invalid episode number has been provided as the argument. A better way than returning NULL (as we will see soon) is to have the resolver function return a 2nd `error` value, but the official Star Wars server return NULL to indicate an invalid episode.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) *Character `egg:"(episode=2)"`
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

In the above code we used an integer to identify episodes.  For example, the `Hero` function's parameter is `episode int`.  For this type of data GraphQL provides enums, which are essentially an integer restricted to a set of named values.  If you are familiar with GraphQL schemas we can define a new `Episode` enum type with 3 values type like this:

```graphqls
enum Episode {
  NEWHOPE
  EMPIRE
  JEDI
}
```
(Don't worry if you are not familiar with schemas as **eggql** hides the details.)

The three allowed values for an `Episode` (NEWHOPE, EMPIRE, and JEDI) are internally represented by the integers 0, 1, and 2.  Because Go does not have a native enum type to support GraphQL enums we just use an integer type *and* also tell **eggql** about the corresponding enum name (eg `Episode`) and the names of its values (eg `NEWHOPE`).  To do this you pass a map of string slices as the 1st (optional) parameter to `MustRun()` where the map key is the enum name and the slice has the enum values.  For example, here is the map for two enums: `Episode` and `LengthUnit`.  (We will use `LengthUnit` a bit later.)

```Go
var gqlEnums = map[string][]string{
	"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
	"LengthUnit": {"METER", "FOOT"},
}
```

It's simple to change the `Hero` resolver to use this `Episode` enum as its argument.

```Go
	Hero func(episode int) *Character `egg:"(episode:Episode=JEDI)"`
```

If you look closely at the above `args` option you can see that the `episode` argument now has a type name after the colon (:) which is the enum name (`Episode`).  The default value is changed from the integer literal `2` to the enum value `JEDI`.

Of course, a resolver can also _return_ an enum, or a list of enums.  Let's add a new `appearsIn` field to the `Character` struct which is a list of the movies that the character appears in.

```Go
	Appears []int `egg:"appearsIn:[Episode]"`
```

Here the first tag option (`appearsIn:[Episode]`) says that the field is called `appearsIn` and the type is a list of `Episode`.  (Square brackets around a type in GraphQL means a list of that type.)

Here's the complete program with the above changes.  Note that the `gqlEnums` map is now the first parameter to `MustRun`.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) *Character `egg:"(episode:Episode=JEDI)"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
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

You will see this result:

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

Interfaces are an advanced, sometimes useful, feature of GraphQL.  Interfaces are a bit like interfaces in the type system of Go, so you may be surprised that **eggql** does not use Go interfaces to implement GraphQL interfaces.  Instead, it uses struct embedding.

To demonstrate interfaces we are going to change the Star Wars example so that the `Character` type is an interface and add two new types `Human` and `Droid` that **implement** the `Character` interface.

```Go
type (
	Human struct {
		Character      // embed Character to use it as a GraphQL "interface"
		Height float64 // only humans have a height (meters)
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
)
```

The above Go code creates two GraphQL types `Human` and `Droid` which implement the GraphQL `Character` interface because they embed the `Character` struct.  (They also have their own type specific fields.)

No changes are required to the earlier `Character` struct, but now it's used as a GraphQL `interface` due solely to the fact that it has been embedded in another struct (or two in this case).

If you have a `Character` struct (or pointer to one) there is no way in Go to find the struct that embeds it or to even determine that it is embedded in another struct.  So to return a `Character` (which is either a `Human` or a `Droid` underneath) we return a Human or Droid as a **Go** `interface{}` and use the **egg** tag (metadata) to indicate that the GraphQL type - see `Character` after the colon (:) in the tage below.

```Go
	Hero func(episode int) interface{} `egg:"hero(episode:Episode=JEDI):Character"`
```

Here the metadata says that the field implements a query called `hero` returning a `Character` (and taking an `episode` argument).  Of course. you also need to change the implementation of the `Hero()` function so that it returns a `Human` or a `Droid` (as an `interface{}`).

This change to the return value causes one further complication that given the `Query` type passed to `MustRun()` there is no way for **eggql** to discover (by reflection) the `Character` type or even the new `Human` and `Droid` types.  The solution is to add a dummy field with a "blank" name of underscore (_).  [Technical note: if you use a zero length array to declare the type, it will take up no space if declared at the start of the struct - eg `_  [0]Character` instead of `_  Character`.  This is usually not important.]

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `egg:"hero(episode:Episode=JEDI):Character"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
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
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
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

Since R2-D2 is a droid you get the `primaryFunction` field.  Now use a different episode as the query parameter, as below.

Note that rather than changing the query in this way it is good practice to "parameterize" any parts that might change using **variables** - see [GraphQL Variables](https://graphql.org/learn/queries/#variables).  But that's outside the scope of this tutorial.

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

Up till now, we have just been using simple queries, so we have omitted the optional `query` keyword at the start of the query.  We'll add it now, because it is required for things like a `mutation` or to add variables.  It also allows naming of queries which can make organising and debugging less confusing.

```graphql
query FriendsOfFriends{
  hero {
    name
    friends {
      name
      friends {
        name
      }
    }
  }
}
```

### Mutations and Input Types

GraphQL is mainly used for receiving information from the backend (server) using queries, but it's sometimes required for a frontend (client) to send information to the backend.  This is what Mutations are for.  Mutations are syntactically identical to queries.  There is a root mutation (by default called `Mutation`) in the same way there is a root query (by default called `Query`). Just as the root query is passed as the 1st parameter to `MustRun()` (2nd parameter if you are using enums) then the root mutation is passed as the next (2nd or 3rd) parameter.

Are mutations necessary? A _query_ could in fact modify data on the backend but that is a bad idea for two reasons:

1. It's confusing to clients (and to backend code maintainers).
2. Mutations are guaranteed to be executed in sequence (whereas a query may resolve in parallel).  If modifications are made using queries the behaviour is undefined and the results unpredictable.

To demonstrate mutations, let's add a `CreateReview` mutation that allows clients to submit movie ratings and reviews.  We add two slices to the existing `EpisodeDetails` struct to store the ratings and reviews.  The root mutation has a `CreateReview` resolver that takes two arguments, an `Episode` (enum) and a `ReviewInput`.  The new `ReviewInput` is used as an **input type** argument to `CreateReview` as explained below.

```Go
type (
	EpisodeDetails struct {
		Name       string
		HeroId     int
		reviewMu   sync.Mutex
		Stars      []int    // rating (0 to 5)
		Commentary []string // review text
	}
	Mutation struct {
		CreateReview func(int, ReviewInput) int `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct { // input type - see below
		Stars      int
		Commentary string
	}
)
```
You may also have noticed we have added a `sync.Mutex` to `EpisodeDetails`.  This requires a quick explanation.

#### Avoiding Data Races

Up until now all the data structures in our example do not change once the server has been started.  Hence, concurrent access from different goroutines does not cause contention.  But now that we have added a mutation this completely changes things.  We have to ensure that multiple mutations running at the same time are not modifying the same data. Even a query reading something while it's being modified can lead to a **data race** condition.

An important thing to remember is that **GraphQL requests may run in parallel**. A GraphQL server would not be of much use if it only processed one client request at a time.  This is not immediately obvious as you don't need to create goroutines because the Go standard library **HTTP handler starts a new goroutine to process every new request**.

Since the `Stars` and `Commentary` slices are modified by the `CreateReview` mutation we need to protect them with a **mutex**.  There are many ways to do this.  We could just use a single mutex, so if a review is being added for any episode then any other query or mutation of _any_ episode is blocked.  This is inefficient, and not very scaleable, so we have used a separate `sync.Mutex` for each episode.  If we were expecting a large number of queries (and few calls to `CreateReview`) a read-write mutex (`sync.RWMutex`) would be the best option, but I'll leave that for you to explore as an exercise.

We only need use the new mutex in `CreateReview` since that is the only place that `Stars` and `Commentary` slices are used, at the moment.

```Go
		CreateReview: func(episode int, review ReviewInput) int {
			if episode < 0 || episode >= len(episodes) { 
				return -1
			}
			episodes[episode].reviewMu.Lock()
			defer episodes[episode].reviewMu.Unlock()
			episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
			episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
			return len(episodes[episode].Stars)-1
		},
```

#### Input Types

Another new thing here is a struct (`ReviewInput`) used as a resolver argument.  (This creates an **input** type in the GraphQL schema.)  An input type is similar to a GraphQL object type except that it can only be used as an argument to a mutation (or query).  Unlike an object (or interface) type the fields of an input type (like `Stars` and `Commentary` above) cannot have arguments, but they can have any type including a nested input type.

Note that if you try to use the same Go struct as an input type _and_ an object (or interface) type then **eggql** will panic with an error like: "can't use Xxx for different GraphQL types (input and object)".

Here is the complete program including the `CreateReview` mutation.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `egg:"hero(episode:Episode=JEDI):Character"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
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
		reviewMu   sync.Mutex
		Stars      []int
		Commentary []string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) int `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
	}
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, Height: 1.67},
		{Character: Character{Name: "Leia Organa"}, Height: 1.65},
		{Character: Character{Name: "Han Solo"}, Height: 1.85},
		{Character: Character{Name: "Chewbacca"}, Height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
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
			CreateReview: func(episode int, review ReviewInput) int {
				if episode < 0 || episode >= len(episodes) { 
					return -1
				}
				episodes[episode].reviewMu.Lock()
				defer episodes[episode].reviewMu.Unlock()
				episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
				episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
				return len(episodes[episode].Stars)-1
			},
		},
	))
	http.ListenAndServe(":8080", nil)
}
```

To run the `CreateReview` mutation send this "query" to the server:

```graphql
mutation {
  createReview(episode: EMPIRE, review: {stars: 5, commentary: "one of the greatest science fiction movies"})
}
```

The response will return the index into the review lists (or -1 if there was an error).  As this is the first review it returns 0.

```json
{
    "data": {
        "createReview": 0
    }
}
```

### Using Go Methods as Resolvers

One of the great things about **eggql** is that you may be able to quickly turn existing software into a GraphQL server.  This is because you can often use existing structs, slices and maps to generate GraphQL types, with trivial changes to your code.  As long as the field name is capitalized then the field will automatically resolve to its value.  A good example of this is the `Height` field of the `Human` struct we have already seen.

One complication with resolver functions is: How do they access the data of their parent?  The first thing is to remember that the Go `func` type is more than a function pointer but a **closure**.  (I guess I should really call them resolver "closures" not resolver functions.) So far we have only assigned a function to a resolver `func` but they can also be assigned instance **method**s, in which case it retains a pointer to the instance.

(I tend to think of a Go `func` variable like a C function pointer, and when you assigned a Go _function_, or `nil`, to it, it *is* essentially just a function pointer.  But it can be _more_ since a closure retains two things: a function pointer and data pointer.)

Imagine we need to change the `height` resolver so that it takes an argument specifying the unit (foot or meter) in which we want the value returned.  To use an argument the `Height float64` field must be converted to a closure that returns a `float64`.

```Go
	Human struct {
		Character
		Height       func(int) float64 `egg:"(unit:LengthUnit=METER)"`
		height       float64            // stored as meters
	}
```

Now the `Height` resolver is a `func` taking a "unit" argument.  The `unit` argument is of type `Unit` - an enum we introduced earlier but did not use.  The original `Height` field is retained but with an initial lower-case 'h' so that it is *not* seen as a GraphQL field. (Remember non-capitalized fields are ignored.)

The problem now is how does the `Height` closure access the `height` field?  To do this we introduce a **method** on the `Human` type which I called `getHeight()`.  This method has the same signature as the `Height` closure (`func(int) float64`).

```Go
func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case 1: // FOOT
		return h.height * 3.28084
	}
	return h.height // METER
}
```

This method is assigned to the `Height` field.  Since we have a slice of `Human`s we have to do it for each element of the slice in the initialisation.

```Go
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	....
	for i := range humans {
		humans[i].Height = (&humans[i]).getHeight // each closure retain an instance pointer to it's human 
	}
```

With this change, if you add a `unit:FOOT` argument to the `height` field of a query like this:

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

### Unions

Unions in GraphQL are like **interfaces**, but without any common fields.  In fact, you can implement the functionality of a union by using an empty interface, except that GraphQL does not allow empty interfaces.  A common use of unions is a query that returns objects of different types without the requirement (as with interfaces) of the objects having anything in common.

In **eggql** you signify a union in the same way as an interface - by embedding a struct in another object (struct), but in this case the embedded `struct` must be empty (in truth, just no exported fields).

In essence, if you embed a `struct` your type implements an interface unless the embedded struct is empty, whence your type is added to a union.

We'll demonstrate this by adding a `search` query as is seen in the standard **Star Wars** tutorial. This allows you to search all humans, droids and starships.  It returns a list of `SearchResult` which is a union of Human | Droid | Starship.  (Note that we won't handle starships in this tutorial but the final code in example\StarWars\main.go does.)

We merely need to create an empty `SearchResult` struct.  Like `Character` we embed it in `Human` and `Droid`.  (We could have had the search return a `Character` except that we later want it also to be able to return other types like `Starship`.)

```Go
type (
    SearchResult struct {} // union SearchResult = Human | Droid
	Human struct {
		SearchResult
		...
	}
	Droid struct {
		SearchResult
		...
	}
)
```

To add the `search` query we need to declare a `Search() func` in the root Query:

```Go
	Query struct {
	    ...
		// Search implements the resolver: "search(text: String!): [SearchResult]"
		Search func(string) []interface{} `egg:"(text):[SearchResult]"`
	}
	....
	http.Handle("/graphql", eggql.MustRun(gqlEnums,
		Query{
		    Hero: ...
		    Search: Search,  // assign Search function to resolver
		},
		Mutation{...
```

`Search()` returns a slice of interface{}, each of which is either a `Human` or a `Droid`.  The `args` option says that the query takes one argument called "text".  The square brackets in the GraphQL return type (`[SearchResult]`) says that it is a list.

The function that implements the `search` query is straightforward.  We just return a list of humans etc. stored in a slice of `interface{}`.

```Go
	http.Handle("/graphql", eggql.MustRun(gqlEnums,
		Query{
		    Hero: ...
			Search: func(text string) (r []interface{}) {
				for _, h := range humans {
					if strings.Contains(strings.ToLower(h.Name), strings.ToLower(text)) {
						r = append(r, h)
					}
				}
				for _, d := range droids {
					if strings.Contains(strings.ToLower(d.Name), strings.ToLower(text)) {
						r = append(r, d)
					}
				}
				return
			},
		},
		Mutation ...
```

The returned objects can be differentiated using inline fragments like this:

```graphql
{
  search(text: "o") {
    ... on Human {
      name
      height
    }
    ... on Droid {
      name
      primaryFunction
    }
  }
}
```

which will return everyone with "o" in their name:

```json
{
    "data": {
        "search": [
            {
                "name": "Han Solo",
                "height": 1.85
            },
            {
                "name": "Leia Organa",
                "height": 1.65
            },
            {
                "name": "C-3PO",
                "primaryFunction": "Protocol"
            }
        ]
    }
}
```

Here's the complete source code at this point.

```Go
package main

import (
	"net/http"
	"strings"
	"sync"

	"github.com/andrewwphillips/eggql"
)

type (
	Query struct {
		_    eggql.TagHolder               `egg:"# The root query object"`
		Hero func(episode int) interface{} `egg:"hero(episode:Episode=JEDI):Character"`
		_    Character
		_    Human
		_    Droid
		// Search implements the resolver: "search(text: String!): [SearchResult]"
		Search func(string) []interface{} `egg:"(text):[SearchResult]"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
	}
	SearchResult struct{} // union SearchResult = Human | Droid
	Human struct {
		Character
		SearchResult
		Height func(int) float64 `egg:"(unit:LengthUnit=METER)"`
		height float64           // stored as meters
	}
	Droid struct {
		Character
		SearchResult
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		reviewMu   sync.Mutex
		Stars      []int
		Commentary []string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) int `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
	}
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
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
			Search: func(text string) (r []interface{}) {
				for _, h := range humans {
					if strings.Contains(strings.ToLower(h.Name), strings.ToLower(text)) {
						r = append(r, h)
					}
				}
				for _, d := range droids {
					if strings.Contains(strings.ToLower(d.Name), strings.ToLower(text)) {
						r = append(r, d)
					}
				}
				return
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) int {
				if episode < 0 || episode >= len(episodes) {
					return -1
				}
				episodes[episode].reviewMu.Lock()
				defer episodes[episode].reviewMu.Unlock()
				episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
				episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
				return len(episodes[episode].Stars) - 1 // new review is the last one
			},
		},
	))
	http.ListenAndServe(":8080", nil)
}

func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case 1:
		return h.height * 3.28084
	}
	return h.height
}
```

### Descriptions

A GraphQL schema can have descriptions for its elements to assist the query designer.  This can be used by tools to interactively build queries - e.g. **PostMan** uses introspection to get the description and display information on query arguments etc.  (In my experience most GraphQL schemas do not contain many, or any, descriptions but adding them can make you service much more useful to its users.)

With **eggql** you can include descriptions to the following GraphQL elements in various ways (as discussed below). The description text always begins with a hash character (#).

* Types - objects, interfaces and unions
* Fields (resolvers) _and_ their argument(s)
* Enum types _and_ their values

#### Types

We use metadata (tags) to attach descriptions to types for adding to the schema.  Unfortunately, Go only allows attaching metadata to **fields** of a `struct`, so to add a description to a type we add a special field to the `struct`  with a name of "_" (single underscore) and a type of `eggql.TagHolder`.  (The `eggql.TagHolder` type has zero size so will not increase the size of your struct if you add it at the top.)  Here is an example using the `Query` struct:

```Go
type Query struct {
	_    eggql.TagHolder `egg:"# The root query object"`
```

This adds the description " The root query object" to the `Query` type in the GraphQL schema.  The same method is used in structs for input, interface and union types.

#### Fields

For resolvers, you just add the description to the tag (at the end of the egg: key string), preceded by a hash character (#).  For example, this adds the description " How tall they are" to the `height` field of the `Human` type.

```Go
	Height  func(int) float64 `egg:"height(unit:LengthUnit=METER) # How tall they are"`
```

#### Arguments

For resolver arguments, just add the description at the end of each argument.  For example, this adds the description "units used for the returned height" to the `unit` argument of the `height` resolver.

```Go
	Height  func(int) float64 `egg:"height(unit:LengthUnit=METER# units used for the returned height)"`
```

#### Enums

Descriptions for enums are done a bit differently since enums are just stored as a slice of strings (since Go does not have enums as part of the language).  For both the enum type's name and the enum values you can add a description to the end of the string, preceded by a hash character (#).  Eg:

```Go
	gqlEnums = map[string][]string{
		"Unit# Units of spatial measurements": {"METER# metric unit", "FOOT# Imperial (US customary) unit"},
```

#### Using Introspection to Obtain Descriptions

You can check the descriptions in the generated schema by using the `GetSchema()` method (see [Viewing the Schema](https://github.com/AndrewWPhillips/eggql#viewing-errors-and-the-schema) in the README). Or you can use introspection to query the running service, for example to get the description of the root query object use this introspection query:

```graphql
{
    __schema {
        queryType {
            name
            description
        }
    }
}
```
which should return:

```JSON
{
  "data": {
    "__schema": {
      "queryType": {
        "name": "Query",
        "description": "The root query object"
      }
    }
  }
}
```

### Error Handling

#### Resolver errors

Resolver `func`s return a single value, but they can optionally return an `error` which will be reported in the "errors" section of the query result.  For example, in the `Hero` function we used above, when there is an error we returned `nil`  which results in a `NULL` `Character` being seen in the GraphQL query results.  An improvement would be to return an error which could provide an explanation.

There are two error conditions in the `Hero` resolver.

1. an invalid episode is supplied as the query parameter - this is an error made by the caller (client)
2. the hero ID stored in the `EpisodeDetails` is invalid - this is an internal error due to data inconsistency

To distinguish between these errors we add a 2nd (`error`) return value to the `Hero` function (see the complete program below).  Now if there is an error the query will return an error message instead of just a `NULL` character.

#### Contexts

A critical part of any server in Go is using the `context.Context` type.  It allows _all_ processing associated with a client request to be expediently and tidily terminated.  A common use is a server request timeout in case anything is taking too long or has completely stalled.

Using **eggql** a resolver function can (optionally) take a 1st parameter of `context.Context`.  You should use a context if the resolver code makes a library or system call that could block on disk or network I/O such as a database query.  You also need a context if you read from or write to a Go `chan` and the other end may block.  Another case is a computationally intensive resolver that can take a long time, in which case you can check if the context has been cancelled regularly (say at least about once a second).

Of course, even if your resolvers are fast and do not block (as in our Star Wars example code), a client may be able to create queries that take a long time to run if using nested queries and/or long lists - eg, a deeply-nested recursive query like:

```graphql
{
    hero {
        name
        friends {
            name
            friends {
               ...
            }
        }
    }
}
```

Fortunately, you do **not** need to handle a context parameter for this as **eggql** will internally cancel the request and not start any more resolvers if the `context` is cancelled.  However, you **do** need to wrap your handler in a timeout handler since Go HTTP handlers do **not** use timeouts by default.  As an example I have added a timeout handler in the code below.  I set a timeout of 5 seconds but if you reduce the timeout (to say 5 milliseconds) then you will be able to see the error generated:

```json
{
  "errors":[
    {
      "message":"timeout"
    }
  ]
}
```

Here is the complete code including the call to `http.TimeoutHandler`. Using `http.TimeoutHandler` and handling `Context` parameters can mitigate problems due to poorly designed client queries, server overload or even a DOS attack.  (Even better is to use **complexity throttling** which analyses the client request and does not try to run queries that exceed a complexity limit.)

```Go
package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andrewwphillips/eggql"
)

type (
	Query struct {
		_      eggql.TagHolder               `egg:"# The root query object"`
		Hero   func(episode int) (interface{}, error) `egg:"hero(episode:Episode=JEDI):Character"`
		_      Character
		_      Human
		_      Droid
		// Search implements the resolver: "search(text: String!): [SearchResult]"
		Search func(string) []interface{} `egg:"(text):[SearchResult]"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
	}
	SearchResult struct{} // union SearchResult = Human | Droid
	Human        struct {
		Character
		SearchResult
		Height func(int) float64 `egg:"(unit:LengthUnit=METER)"`
		height float64           // stored as meters
	}
	Droid struct {
		Character
		SearchResult
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		reviewMu   sync.Mutex
		Stars      []int
		Commentary []string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) int `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
	}
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2000},
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
	handler := eggql.MustRun(gqlEnums,
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
			Search: func(text string) (r []interface{}) {
				for _, h := range humans {
					if strings.Contains(strings.ToLower(h.Name), strings.ToLower(text)) {
						r = append(r, h)
					}
				}
				for _, d := range droids {
					if strings.Contains(strings.ToLower(d.Name), strings.ToLower(text)) {
						r = append(r, d)
					}
				}
				return
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) int {
				if episode < 0 || episode >= len(episodes) {
					return -1
				}
				episodes[episode].reviewMu.Lock()
				defer episodes[episode].reviewMu.Unlock()
				episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
				episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
				return len(episodes[episode].Stars) - 1
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
	switch unit {
	case 1:
		return h.height * 3.28084
	}
	return h.height
}
```

### Custom Scalars

GraphQL supports the creation of custom scalar types.  You can easily add custom scalars in **eggql** which we'll demonstrate using a `ReviewTime` type, adding a field to the `Review` input type to record when a movie review was written.  Note that we could use the similar `eggql.Time` which implements the GraphQL `Time` type, but we'll create our own custom scalar type to demonstrate how it's done.

All that is required to create a custom scalar is to define a Go type that implements the `UnmarshalEGGQL` method.  This method is used to convert a specially formatted string into a value of your type.  You may also want to implement a `MarshalEGGQL` method that performs the reverse operation, but some types can get by without this (e.g. if they implement a `String` method).

In our case we will create a new struct type called `ReviewTime` that uses the standard Go `time.Time` type internally.  Here is the complete code for the new type:

```Go
type ReviewTime struct{ time.Time }

// UnmarshalEGGQL is called when eggql needs to decode a string to a ReviewTime
func (prt *ReviewTime) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse(time.RFC3339, in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time decoding %q", err, in)
	}
	prt.Time = tmp
	return nil
}
```

Note: for your type to be used as a custom scalar **you must provide a method with this exact signature**: `UnmarshalEGGQL(string) error`.

For the above type we don't need to provide the inverse `MarshallEGGQL` method. The `ReviewTime` type has all the methods of `time.Time` because it is *embedded* in it, including the `String() string` method which is used for "marshalling".  But if we did provide a method it **must have this signature**: `MarshalEGGQL() (string, error)`.

To use the new type we just add a new `ReviewTime` field to `ReviewInput` and `EpisodeDetails`.

```Go
	EpisodeDetails struct {
		Name       string
		HeroId     int
		reviewMu   sync.Mutex
		Stars      []int
		Commentary []string
		Time       []ReviewTime // *** new
	}
	// ...
	ReviewInput struct {
		Stars      int
		Commentary string
		Time       *ReviewTime // *** new
	}
```

Since `Time` is a pointer in `ReviewInput` it is nullable which means that it does not need to be provided.  In the `CreateReview` code we use the **current time** if it's not specified.

```Go
    CreateReview: func(episode int, review ReviewInput) *EpisodeDetails {
        // ...
        if review.Time == nil {
            episodes[episode].Time = append(episodes[episode].Time, ReviewTime{time.Now()})
        } else {
            episodes[episode].Time = append(episodes[episode].Time, *review.Time)
        }
        return len(episodes[episode].Stars)-1
    },
```

### Subscriptions

Subscriptions are one of the most powerful features of GraphQL. They allow a client to request a stream of data that is sent over a permanent connection (ie, websocket).  Setting up Subscriptions is easy - almost exactly the same as Queries and Mutations.  For example, there is a root Subscription object, just like the root Query object.  The major difference with subscriptions is that the resolvers do not return a value - instead they return a **Go channel** which sends a stream of values.

In this section we will allow the client to subscribe to new reviews for an episode.

The (new) root Subscription object has one `NewReviews` resolver that returns a channel of reviews.  It takes two parameters: a context that is used to detect when the subscription is closed, and an enum used as the subscription parameter to determine which reviews we want.

We need a new `Review` type to send details of the new review.  Note that although this has the same fields as the `ReviewInput` struct, we need a new struct as we can't use the same Go struct as a GraphQL `input` type and `object` type.

```Go
	Subscription struct {
		NewReviews func(context.Context, int) <-chan Review `egg:"(episode:Episode)"`
	}
	Review struct {
		Stars      int
		Commentary string
		Time       *ReviewTime
	}
```

To keep track of all subscribers for reviews we need a new field in the `EpisodeDetails` struct:

```Go
	EpisodeDetails struct {
	    ...
	    reviewReceivers map[chan<- Review]context.Context
	}
```

The only code changes are to implement `NewReviews`, returning a channel, and modify the `CreateReview` mutation so that the new review is posted to all subscribers

```Go
	NewReviews: func(ctx context.Context, episode int) <-chan Review {
		ch := make(chan Review)
		episodes[episode].reviewMu.Lock()
		defer episodes[episode].reviewMu.Unlock()
		episodes[episode].reviewReceivers[ch] = ctx
		return ch
	},
	...
	CreateReview: func(episode int, review ReviewInput) int {
	    ...
		if len(episodes[episode].reviewReceivers) > 0 {
			out := Review{
				Stars:      review.Stars,
				Commentary: review.Commentary,
				Time:       review.Time,
			}
			for ch, ctx := range episodes[episode].reviewReceivers {
				if ctx.Err() != nil {
					delete(episodes[episode].reviewReceivers, ch)
					close(ch)
					continue
				}
				ch <-out
			}
		}
		return len(episodes[episode].Stars) - 1
	},
```

### Conclusion

I trust this tutorial has helped you to see how easy it is to create a simple GraphQL server using **eggql**.  You don't have to create, or even understand GraphQL schemas.  (Under the hood, a schema is generated for you which you can view if you need to.)  Unlike other Go packages, this avoids getting lots of run-time panics when your schema does not match your data types.

However, **eggql** may not be the best solution for you if you want something comprehensive or more efficient.  It does not have any support for databases, such as a dataloader since I wrote it to work with in-memory data.  It may also be too slow for heavy load as it uses reflection.  See the README for some excellent alternative Go GraphQL packages.

------
For your reference, here is the final code, as at the end of the tutorial.  Further enhancements may be found at https://github.com/AndrewWPhillips/eggql/tree/main/example/starwars.

```Go
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andrewwphillips/eggql"
)

// ReviewTime implements a GraphQL custom scalar used to keep track of when a movie review was posted
type ReviewTime struct{ time.Time } // embed Time so we get String() method (for marshaling)

// UnmarshalEGGQL is called when eggql needs to decode a string to a Time
// The existence of this method signals that this type is a custom scalar/
func (rt *ReviewTime) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse(time.RFC3339, in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time", err)
	}
	rt.Time = tmp
	return nil
}

type (
	Query struct {
		_    eggql.TagHolder                        `egg:"# The root query object"`
		Hero func(episode int) (interface{}, error) `egg:"hero(episode:Episode=JEDI):Character"`
		_    Character
		_    Human
		_    Droid
		// Search implements the resolver: "search(text: String!): [SearchResult]"
		Search func(string) []interface{} `egg:"(text):[SearchResult]"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
	}
	SearchResult struct{} // union SearchResult = Human | Droid
	Human        struct {
		Character
		SearchResult
		Height func(int) float64 `egg:"(unit:LengthUnit=METER)"`
		height float64           // stored as meters
	}
	Droid struct {
		Character
		SearchResult
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name            string
		HeroId          int
		reviewMu        sync.Mutex
		Stars           []int
		Commentary      []string
		Time            []ReviewTime
		reviewReceivers map[chan<- Review]context.Context
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) int `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
		Time       *ReviewTime `egg:"# time the review was written - current time is used if NULL"`
	}

	Subscription struct {
		NewReviews func(context.Context, int) <-chan Review `egg:"(episode:Episode)"`
	}
	Review struct {
		Stars      int
		Commentary string
		Time       *ReviewTime
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"},
	}
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2000},
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
	handler := eggql.MustRun(gqlEnums,
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
			Search: func(text string) (r []interface{}) {
				for _, h := range humans {
					if strings.Contains(strings.ToLower(h.Name), strings.ToLower(text)) {
						r = append(r, h)
					}
				}
				for _, d := range droids {
					if strings.Contains(strings.ToLower(d.Name), strings.ToLower(text)) {
						r = append(r, d)
					}
				}
				return
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) int {
				if episode < 0 || episode >= len(episodes) {
					return -1
				}
				episodes[episode].reviewMu.Lock()
				defer episodes[episode].reviewMu.Unlock()
				episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
				episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
				if review.Time == nil {
					episodes[episode].Time = append(episodes[episode].Time, ReviewTime{time.Now()})
				} else {
					episodes[episode].Time = append(episodes[episode].Time, *review.Time)
				}
				if len(episodes[episode].reviewReceivers) > 0 {
					out := Review{
						Stars:      review.Stars,
						Commentary: review.Commentary,
						Time:       review.Time,
					}
					for ch, ctx := range episodes[episode].reviewReceivers {
						if ctx.Err() != nil {
							delete(episodes[episode].reviewReceivers, ch)
							continue
						}
						ch <- out
					}
				}
				return len(episodes[episode].Stars) - 1
			},
		},
		Subscription{
			NewReviews: func(ctx context.Context, episode int) <-chan Review {
				ch := make(chan Review)
				episodes[episode].reviewMu.Lock()
				defer episodes[episode].reviewMu.Unlock()
				episodes[episode].reviewReceivers[ch] = ctx
				return ch
			},
		},
	)
	//handler = http.TimeoutHandler(handler, 5*time.Second, `{"errors":[{"message":"timeout"}]}`)
	http.Handle("/graphql", handler)
	http.ListenAndServe(":8080", nil)
}

// getHeight returns the height of a human
// Parameters
//  h (receiver) is a pointer to the Human
//  unit is the unit for the return value
func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case 1:
		return h.height * 3.28084
	}
	return h.height
}
```
