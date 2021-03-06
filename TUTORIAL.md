## Star Wars Tutorial

This tutorial will show you how to implement a GraphQL service using Go and **eggql**.  (See the corresponding GraphQL [Star Wars Frontend Tutorial](https://graphql.org/learn/) for how you can query this service.)  It shows how to implement every feature of a GraphQL service except for subscriptions.  (Subscriptions are also not covered in the **Star Wars** frontend tutorial.)

Remember, this tutorial is about implementing the backend (a GraphQL service), so I'll focus on creating the service, not on using it.  It's mainly Go code with a few test queries.  We **won't** look at aliases, fragments, inline fragments, variables, directives, schemas and introspection, but rest assured they work.  These things are all covered in the GraphQL **Star Wars** frontend tutorial if you want to try the queries from that.

You should probably follow this tutorial sequentially as each section builds on the previous, but here are some links if you need to quickly find specific information:

1. [Basic Types](#basic-types) - objects (including nested objects), lists and scalars (String, Int, etc)
2. [Resolvers and Arguments](#resolver-functions-and-arguments) - a resolver function can have argument(s) to refine the data returned
3. [Enums](#enums) - GraphQL has enum types but Go doesn't, so they need to be handled specially
4. [Mutations](#mutations-and-input-types) - Queries retrieve data, Mutations modify the data
5. [Input Types](input-types) - mutations (and queries) sometimes need complex arguments
6. [Interfaces](#interfaces) - similar objects can implement an interface to indicate a common behaviour (polymorphism)
7. [Unions](#unions) - like interfaces except the types in the union have no fields in common
8. [Descriptions](#descriptions) - fields, arguments, etc can have a description to be used by query designers
9. [Errors](#resolver-errors) - how to handle errors
10. [Contexts](#contexts) - contexts are used to cancel GraphQL queries - eg for a timeout if they take too long to run
11. [Methods as Resolvers](#using-go-methods-as-resolvers) - how a resolver function can easily access data of its parent object
12. [Custom Scalars](#custom-scalars) - you can add new types to be used in your queries called custom scalars

Note: the final code for this tutorial is in the git repo (https://github.com/AndrewWPhillips/eggql/tree/main/example/starwars).  It's less than 250 lines of code (and most of that is just data field initialization since all the data is stored in memory).  This is much smaller (and I think much simpler) than the equivalent Star Wars example using other GraphQL packages (for Go and even other languages).  The complete version is also running *right now* in GCP (Google Cloud Platform), so you can try any Star Wars queries in Postman (or Curl etc) using the address https://aphillips801-eggql-sw.uc.r.appspot.com/graphql.

### Basic Types

GraphQL is all about types - scalar types (int, string, etc), object types which are composed of fields of other types (a bit like Go structs), lists (a bit like a Go slices) and more specialized types like interfaces, unions and input types (which we will get to later).

Traditionally when building a GraphQL service, you first create a schema which defines your types, but with **eggql** you just use Go structs; *the schema is created for you*.  (To see the generated schema refer to [Viewing the Schema](https://github.com/AndrewWPhillips/eggql#viewing-errors-and-the-schema) in the README.)  The first thing you need is a **root query** which is just like any other GraphQL object type.  The root query **fields** define the queries that can be submitted to the GraphQL server.

First we'll add queries returning basic types (scalars, lists and nested objects).  Then we'll look at how to implement query arguments, mutations, and more advanced types.  We'll also look at the sorts of errors you can get and how to handle them.

Here's a Go program that will handle the first (`hero`) query of the GraphQL Star Wars tutorial (see https://graphql.org/learn/queries/).

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

Here the type `Query` is the root query as an object of its type is the first (only) parameter passed to `MustRun()`.  With this service running you can post a `hero` query to the server which returns a `Character`.  The `Character` object can be queried for its name and for a list of friends.  Try this query:

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

Note that you could recursively query the friends of the friends.  You can even query friends of friends of friends ... to any depth, but it is unwise to nest queries too deeply (and some servers limit nesting to 3 or 4 levels).  You can't see this with the above data as Luke and Leia sadly do not have any friends yet.

The `Character` type is an object since it has fields (sub-queries) within it. `Query` is an object, but it is special being the **root query**.

The `Friends` field of `Character` defines a list, in this case implemented using a slice of pointers.

The `Name` field has the GraphQL scalar type of `String!` because it uses the Go `string` type.  Similarly, any Go integer types create the GraphQL `Int!` type, Go bool => `Boolean!` and float32/float64 => `Float!`.  Note that these types are followed by an exclamation mark (!) which indicates they are not nullable.  You can get the nullable version by using pointers or the `nullable` tag.

Now we'll look at some more advanced types....

### Resolver Functions and Arguments

In GraphQL the server code that processes a query is called a **resolver**.  In the above example the "resolver" for the `hero` query was just a `Character` struct.  A more useful and more common thing is for a resolver to be a function (technically a Go **closure**).  Using functions for resolvers has advantages, as it allows:

* efficiency, since the function is not executed unless or until it's specific data is required
* use of values which are initially unknown or dynamic (eg random numbers as in the README example)
* recursive queries (eg friends of friends of ...), which can't be pre-computed (without infinite recursion)
* resolvers can take arguments that can modify or refine the results returned

It's the last advantage that we look at here - resolver arguments.  As an example we can change the `hero` resolver to be a function that takes a parameter specifying which episode we want the hero for.  So instead of the `Hero` field just being a `Character` object it is now a function that _returns_ a `Character`.

```Go
type Query struct {
	Hero func(episode int) Character `egg:"hero(episode=2)"`
}
```
This also shows our first use of the **egg: key** taken from the `Hero` field's **tag string**.  This sort of metadata in Go can be attached to any field of a struct by adding a string after the field declaration.

You can have several (comma-separated) options in the tag string but the first one is the most important as it allows you to specifiy the resolver name, its type, and it's arguments (name/type/default value)
The first option in the `egg:` metadata is the resolver name - in this case `hero`.  We don't really need to supply the name, in this case, as it defaults to the field name (`Hero`) with the first letter converted to lower-case.

In brackets after the name (eg. `(episode=2)`) are the resolver argument(s). The number of arguments (comma-separated) in the `args` option must match the number of function parameters (except that the function may also include an initial `context.Context` parameter as discussed later).  The names of the argument(s) must be given in the `args` (in this case there is just one called `episode`) and you can also give an optional default value (in this case `2`). [Technical note: you always need to supply a name since we can't obtain the argument name from the `Hero` function parameter name as Go reflection only provides the types of function parameters not their names.]

Here is a complete program with the `Hero` resolver.  Note that I changed the `Hero()` function to return a pointer to `Character`; this allows us to return a null value when an invalid episode number has been provided as the argument. A better way than returning NULL (as we will see soon) is to have the resolver function return a 2nd `error` value, but the official Star Wars server returns NULL like this.

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
(Don't worry if you are not familiar with schemas as **eggql** will generate the schema automatically.)

The three allowed values for an `Episode` (NEWHOPE, EMPIRE, and JEDI) are internally represented by the integers 0, 1, and 2.  Because Go does not have a native enum type to support GraphQL enums we just use an integer type *and* also tell **eggql** about the corresponding enum name (eg `Episode`) and the names of its values (eg `NEWHOPE`).  To do this you pass a map of string slices as the 1st (optional) parameter to `MustRun()` where the map key is the enum name and the slice has the enum values.  For example, here is the map for two enums: `Episode` and `Unit`.

```Go
var gqlEnums = map[string][]string{
	"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
	"Unit":    {"METER", "FOOT"},
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

Here the first tag option (`appearsIn:[Episode]`) says that the GraphQL name of the field is `appearsIn` and the type is a list of `Episode`.  (Square brackets around a type in GraphQL means a list.)

Here's the complete program with the above changes.

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

Interfaces are an advanced, but sometimes useful, feature of GraphQL.  Interfaces are a bit like interfaces in the type system of Go, so you may be surprised that **eggql** does not use Go interfaces to implement GraphQL interfaces.  Instead, it uses struct embedding.

To demonstrate interfaces we are going to change the Star Wars example so that the `Character` type is an interface and add two new types `Human` and `Droid` that **implement** the `Character` interface.

```Go
type (
	Human struct {
		Character      // embed Character to use it as a GraphQL "interface"
		Height float64 // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
)
```

The above Go code creates two GraphQL types `Human` and `Droid` which implement the `Character` interface because the Go struct embeds the `Character` struct.  (They also have their own type specific fields.)

No changes are required to the earlier `Character` struct, but now it's used as a GraphQL `interface` due solely to the fact that it has been embedded in another struct (or two in this case).

If you have a Character struct (or pointer to one) there is no way in Go to get the struct that embeds it or to even determine that it is embedded in another struct.  So to return a `Character` (which is either a `Human` or a `Droid` underneath) we return a Human or Droid as a **Go** `interface{}` and use metadata to indicate that the GraphQL type is a `Character` interface.

```Go
	Hero func(episode int) interface{} `egg:"hero(episode:Episode=JEDI):Character"`
```

Here the first option in the egg: key's metadata (`hero:Character`) says that the field is called `hero` and its type is `Character`.  Of course. you also need to change the implementation of the Hero() function so that it returns a `Human` or a `Droid` (as an `interface{}`).

This change to the return value causes one further complication that given the `Query` type passed to `MustRun()` there is no way for **eggql** to discover (by reflection) the `Character` type or even the new `Human` and `Droid` types.  The solution is to add a dummy field with a "blank" name of underscore (_).  [Technical note: if you use a zero length array to declare the type, it will take up no space if declared at the start of the struct - eg `_  [0]Character` instead of `_  Character`.  This is usually not important.]

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `egg:"hero:Character(episode:Episode=JEDI)"`
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

Since R2-D2 is a droid you get the `primaryFunction` field.  Now use a different episode as the query parameter, as below.

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

GraphQL is mainly used for receiving information from the backend (server) using queries, but it's sometimes required for a frontend (client) to _modify_ information on the backend.  This is what Mutations are for.  Mutations are syntactically identical to queries.  There is a root mutation (by default called `Mutation`) in the same way there is a root query (by default called `Query`). Just as the root query is passed as the 1st parameter to `MustRun()` (2nd parameter if you have enums) then the root mutation is passed as the next (2nd or 3rd parameter).

Are mutations necessary? A _query_ could in fact modify data on the backend but that is a bad idea for two reasons:

1. It's confusing to clients (and to backend code maintainers).
2. Mutations are guaranteed to be executed in sequence (whereas a query may resolve in parallel).  If modifications are made using queries the behaviour is undefined and the results unpredictable.

To demonstrate mutations, let's ad one that allows clients to submit movie ratings and reviews.  Here are the relevant Go types, including the root mutation `struct`:

```Go
type (
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
	}
	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `egg:"(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)
```
We've already seen the `EpisodeDetails` struct, but it now has two new fields (`Stars` and `Commentary`) which are used to save the submitted values from the client.  The root mutation has a `CreateReview` mutation that takes two arguments, an `Episode` (enum) and a `ReviewInput`.  It returns the `EpisodeDetails` to confirm the change, but can also return NULL (since the return type is a **pointer** to `EpisodeDetails`) if there is an error such as an invalid episode.

#### Input Types

A new thing here is a struct (`ReviewInput`) used as a resolver argument.  This creates a GraphQL **input** type.  An input type is similar to a GraphQL object type except that it can only be used as an argument to a mutation or query.  Unlike an object (or interface) type the fields of an input type cannot have arguments.  Also, if you try to use the same Go struct as an input type _and_ an object (or interface) type then **eggql** will return an error.  TODO: include error message

Here is the complete program with the `CreateReview` mutation.

```Go
package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `egg:"hero:Character(episode:Episode=JEDI)"`
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
		Stars      int
		Commentary string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `egg:"(episode:Episode,review)"`
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

One of the great things about **eggql** is that you may be able to quickly turn existing software into a GraphQL server.  This is because you can often use existing structs, slices and maps to generate GraphQL types, with trivial changes to your code.  As long as the field name of a struct is capitalized then the field will automatically resolve to its value.  A good example of this is the `height` field of the `Human` struct we have already seen.

One complication with resolver functions is: How do they access the data of their parent?  The first thing is to remember that the Go `func` type is more than a function pointer but a **closure**.  (I guess I should really call them resolver "closures" not resolver functions.) So far we have only assigned a function to a resolver `func` but (as it's a closure) it can also be assigned an instance **method**, in which case it retains a pointer to the instance.

(I tend to think of a Go `func` variable like a C function pointer, and when you assigned a Go _function_, or `nil`, to it, it *is* essentially just a function pointer.  But it can be _more_ since a closure retains two things: a function pointer and data pointer.)

Imagine we need to change the `height` resolver so that it takes an argument specifying the unit (foot or meter) in which we want the value returned.  To use an argument the `Height float64` field must be converted to a closure that returns a `float64`.

```Go
	Human struct {
		Character
		Height       func(int) float64 `egg:"(unit:Unit=METER)"`
		height       float64            // meters
	}
```

Now the `Height` resolver is a `func` taking a "unit" argument.  The `unit` argument is of type `Unit` - an enum we introduced earlier but did not use.  The original `Height` field is retained but with an initial lower-case 'h' so that it is *not* seen as a GraphQL field.

The problem now is how does the `Height` closure access the `height` field?  To do this we introduce a **method** on the `Human` type which I called `getHeight()`.  This method has the same signature as the `Height` closure (`func(int) float64`).

```Go
func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case FOOT:
		return h.height * 3.28084
	}
	return h.height
}
```

This method is assigned to the `Height` field.  Since we have a slice of `Human`s we have to do it for each element of the slice in the initialisation.

```Go
	for i := range humans {
		humans[i].Height = (&humans[i]).getHeight
	}
```

With this change, if you add a `unit:FOOT` argument to the `height` sub-query like this:

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

Unions in GraphQL are like **interfaces**, but without any common fields.  In fact, you can implement the functionality of a union by using an empty interface, except that GraphQL does not allow empty interfaces.  A common use of unions is a query that returns objects of different types without the requirement (as with interfaces) of anything in common.

In **eggql** you signify a union in the same way as an interface - by embedding a struct in another object (struct).  But for a union the embedded `struct` can't have exported fields.

In essence, if you embed a `struct` your type implements an interface unless the embedded struct is empty, whence your type is added to a union.

We'll demonstrate this by adding a `search` query as is seen in the standard **Star Wars** tutorial. This allows you to search all humans, droids and starships.  It returns a list of `SearchResult` which is a union of Human | Droid | Starship.  (Note that we won't handle starships in this tutorial but the final code in example\StarWars\main.go does.)

Here's the relevant change to the data:

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
		Search func(string) []interface{} `egg:":[SearchResult],args(text)"`
	}
```

`Search()` returns a slice of interface{}, each of which is either a `Human` or a `Droid`.  The `args` option says that the query takes one argument called "text".  The square brackets in the GraphQL return type (`[SearchResult]`) says that it is a list.

The function that implements the `search` query is straightforward.  We just return a list of humans etc stored in a slice of `interface{}`.

```Go
func Search(text string) (r []interface{}) {
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
```

The returned objects can be differentiated using inline fragments like this:

```
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

which will list everyone with "o" in their name:

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

### Descriptions

A GraphQL schema can have descriptions for many of its elements.  This is for documentation but can be used by tools used to interactively build queries - eg **PostMan** uses introspection to get the description and display information on query arguments etc.  (In my experience most GraphQL schemas do not contain many, or any, descriptions but adding them can make you service far more usable.)

With **eggql** you can include descriptions to the following GraphQL elements in various ways (as discussed below) but the description text always begins with a hash character (#).

* Types - objects, interfaces and unions
* Fields (resolvers) and their argument(s)
* Enum types and each of their values

#### Types

We use metadata (tags) to attach descriptions to types for adding to the schema.  Unfortunately Go only allows attaching metadata to **fields** of a `struct`, so to add a description we add a special field to the `struct`  with a name of "_" (single underscore) and a type of `eggql.TagHolder`.  (This type will not increase the size of your struct if you add it at the top.)  Here is an example using the `Query` struct:

```Go
type Query struct {
	_    eggql.TagHolder `egg:"# The root query object"`
```

This adds the description " The root query object" to the `Query` type in the GraphQL schema.  The same method is used in structs for input, interface and union types.

#### Fields

For resolvers you just add the description preceded by a hash character (#) to the end of the egg: key's string.  For example, this adds the description " How tall they are" to the `height` field of the `Human` type.

```Go
	Height  func(int) float64 `egg:"height(unit:LengthUnit=METER) # How tall they are"`
```

#### Arguments

For resolver arguments, just add the description at the end of each argument using the `args` option of the `graphql` tag.  For example, this add "units used for the returned height" to the `unit` argument of the `height` resolver.

```Go
	Height  func(int) float64 `egg:"height(unit:LengthUnit=METER# units used for the returned height)"`
```

#### Enums

Descriptions for enums are done a bit differently since enums are just stored as a slice of strings (since Go does not have enums as part of the language).  For both the enum type's name and the enum values you can add a description after the name.  Eg:

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

Resolver `func`s return a single value, but they can optionally return an `error` which will be reported in the "errors" section of the query result.  For example, in the `Hero` function we used above, when there is an error we return `nil`  which results in a `NULL` `Character` being seen in the GraphQL query results.  An improvement would be to give an explanation of the cause of the error.

There are two error conditions in the `Hero` resolver.

1. an invalid episode is supplied as the query parameter - this is an error made by the caller (client)
2. the hero ID stored in the `EpisodeDetails` is invalid - this is an internal error due to data inconsistency

To distinguish between these errors we add a 2nd (`error`) return value to the `Hero` function (see the complete program below).  Now if there is an error the query will return an error message instead of just a `NULL` character.

#### Contexts

A critical part of any server in Go is using the `context.Context` type.  It allows _all_ processing associated with a client request to be expediently and tidily terminated.  This is most commonly used in web servers for a timeout in case anything is taking too long or has completely stalled.

Using **eggql** a resolver function can (optionally) take a 1st parameter of `context.Context`.  You would almost certainly use a context if the resolver code read from or wrote to a Go `chan`, or made a library or system call that could block on disk or network I/O such as a database query.  A less common scenario is a computationally intensive resolver in which case you can check if the context has been cancelled regularly, such as in an inner loop.

Our Star Wars example works with in-memory data structures so the resolver functions do _not_ need context parameters.  (See the [Getting Started](https://github.com/AndrewWPhillips/eggql#getting-started) example in the README where a `context` parameter is added to the `random` query.)  Even so, since GraphQL queries can return lists and nested queries, a single GraphQL request can cause a cascade of queries taking a long time even if each individual query does not - e.g. if you deeply nested a friends query like this:

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

Fortunately, **eggql** itself will automatically not start any more query processing if the `context` is cancelled.  If you test this with a deeply nested query (and perhaps reduce the timeout in the `TimeoutHandler` below), you will see a message like this, even without `Hero` using a `Context` parameter:

```json
{
  "errors":[
    {
      "message":"timeout"
    }
  ]
}
```

Note that Go HTTP handlers do **not** have timeouts by default, so the GraphQL handler is wrapped in a timeout handler (see the call to `http.TimeoutHandler()`) which creates a context that expires after 5 seconds.  Using a `Context` like this can mitigate problems due to poorly designed client queries, server overload or even a DOS attack.

```Go
package main

import (
	"fmt"
	"github.com/andrewwphillips/eggql"
	"net/http"
	"strings"
	"time"
)

type (
	Query struct {
		Hero func(episode int) (interface{}, error) `egg:"hero:Character,args(episode:Episode=JEDI)"`
		_    Character
		_    Human
		_    Droid
		Search func(string) []interface{} `egg:"(text):[SearchResult]"`
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `egg:"appearsIn:[Episode]"`
	}
	SearchResult struct{}
	Human struct {
		SearchResult
		Character
		Height func(int) float64 `egg:"(unit:Unit=METER)"`
		height float64           // meters
	}
	Droid struct {
		SearchResult
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
		CreateReview func(int, ReviewInput) *EpisodeDetails `egg:",args(episode:Episode,review)"`
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

### Custom Scalars

GraphQL supports the creation of custom scalar types.  You can easily add custom scalars in **eggql** which we'll demonstrate using a `ReviewTime` type, adding a field to the `Review` input type to record when a movie review was written.  Note that we could use the similar `eggql.Time` which implements the `Time` scalar type, but we'll create our own custom scalar type to demonstrate how it's done.

All that is required to create a custom scalar is to define a Go type that implements the `UnmarshalEGGQL` method.  This method is used to convert a specially formatted string into a value of your type.  You may also want to implement a `MarshalEGGQL` method that performs the reverse operation, but some types can get by without this (eg if they implement a `String` method).

In our case we will create a new struct type called `ReviewTime` that uses standard Go `time.Time` internally.  Here is the complete code for the new type:

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

For the above type we don't need to provide the inverse `MarshallEGGQL` method.  Because `time.Time` is an unnamed field it is *embedded* in `ReviewTime` which means the `ReviewTime` type "inherits" all the methods of `time.Time` including the `String() string` method which is used for "marshalling".  But if we did provide a method it **must have this signature**: `MarshalEGGQL() (string, error)`.

To use the new type we just add a new `ReviewTime` field to `ReviewInput` and `EpisodeDetails`.

```Go
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
		Time       ReviewTime
	}
	// ...
	ReviewInput struct {
		Stars      int
		Commentary string
		Time       *ReviewTime
	}
```

Since `Time` is a pointer in `ReviewInput` it is nullable which means that it does not need to be provided.  In the `CreateReview` mutation we use the **current time** if it's not specified.

```Go
    CreateReview: func(episode int, review ReviewInput) *EpisodeDetails {
        // ...
        if review.Time == nil {
            episodes[episode].Time = append(episodes[episode].Time, ReviewTime{time.Now()})
        } else {
            episodes[episode].Time = append(episodes[episode].Time, *review.Time)
        }
        return &episodes[episode]
    },
```

### Conclusion

I trust this tutorial has helped you to see how easy it is to create a simple GraphQL server using **eggql**.  You don't have to create, or even understand GraphQL schemas.  (Under the hood, a schema is generated for you which you can view if you need to.)  Unlike other Go packages this avoids getting lots of run-time panics when your schema does not match your data types.

However, **eggql** may not be the best solution for you if you want something comprehensive or more efficient.  It does not have any support for databases, such as a dataloader since I wrote it to work with in-memory data.  It may also be too slow for heavy load as it uses reflection and linear searches to run resolvers.  See the README for some excellent alternative Go GraphQL packages.
