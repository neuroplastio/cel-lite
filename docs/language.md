---
group: Reference
order: 10
nav: The language
description: what the subset of CEL holds, what cel-lite refuses and how, and where it differs from cel-go
---

# The language

cel-lite runs a subset of [CEL](https://cel.dev), the Common Expression
Language. An expression means what CEL's
[language definition](https://github.com/google/cel-spec/blob/master/doc/langdef.md)
says it means. What cel-lite doesn't implement it refuses, by name, when
the expression is compiled: an expression that compiles never means
something other than it would in CEL.

[cel-go](https://github.com/google/cel-go), CEL's reference
implementation, is the oracle. The tests in `oracle/` run the same
expressions through both engines and compare types, values, errors and
unknowns: a corpus of corner cases, thousands of generated expressions on
random variables, and two fuzz targets. Where cel-go strays from the
definition, cel-lite follows the definition, and the difference is listed
[below](#where-cel-lite-and-cel-go-differ).

## Types

| Type          | Values                                    | Go                    |
| ------------- | ----------------------------------------- | --------------------- |
| `bool`        | `true`, `false`                           | `bool`                |
| `int`         | 64-bit signed integers                    | `int64`               |
| `uint`        | 64-bit unsigned integers                  | `uint64`              |
| `double`      | 64-bit floating point                     | `float64`             |
| `string`      | Unicode text                              | `string`              |
| `bytes`       | byte sequences                            | `[]byte`              |
| `list(T)`     | `[1, 2, 3]`                               | a slice, or a `List`  |
| `map(K, V)`   | `{'a': 1}`; keys bool, int, uint, string  | a map, or a `Map`     |
| `null_type`   | `null`                                    | `nil`                 |
| a struct      | a struct your program declares            | an `Object`           |
| `dyn`         | any of the above, checked as it runs      |                       |

Every expression is type-checked when it compiles. A value of type `dyn`
is checked when the expression runs instead: `dyn(x)` makes any value
dyn, and a variable declared dyn holds anything.

## Literals

```
42  -7  0x1F            int
42u  0x1Fu              uint
1.5  1e3  .5  1.e3      double
'single'  "double"      string, with \n \t \\ \' \" \xHH \uHHHH escapes
'''triple'''  """too""" string, across lines
r'raw \n'               string, no escapes
b'bytes\xff'            bytes
true  false  null
[1, 2, 3]  {'a': 1}     list, map; a trailing comma is allowed
```

## Operators

From the loosest to the tightest:

| Operators                          | Meaning                                  |
| ---------------------------------- | ---------------------------------------- |
| `c ? a : b`                        | `a` if `c`, else `b`; `c` is a bool      |
| `\|\|`                             | or                                       |
| `&&`                               | and                                      |
| `==` `!=` `<` `<=` `>` `>=` `in`   | equality, order, membership              |
| `+` `-`                            | add, subtract, concatenate               |
| `*` `/` `%`                        | multiply, divide, remainder              |
| `!` `-`                            | not, negate                              |
| `a.f` `a[i]` `f(x)` `a.f(x)`       | select, index, call, method call         |

- **Arithmetic** takes two values of one type: `1 + 1u` is a type error.
  Overflow in `int` or `uint` is an error when the expression runs, and so
  is dividing an integer by zero. `double` follows IEEE 754: `1.0 / 0.0`
  is infinity, and NaN equals nothing, itself included. Integer division
  truncates toward zero: `-5 / 3` is `-1`, and `-5 % 3` is `-2`.
- **`+`** also concatenates strings, bytes and lists.
- **Equality and order** take two values of one type when their types
  are known: `1 == 1u` is a type error, as in cel-go. Values of type dyn
  compare across the numeric types by their value: `dyn(1) == 1u` and
  `dyn(1) < 1.5` hold. Lists and maps are equal when their elements are.
  Strings order by code point, bytes by byte, and `false < true`.
- **`in`** is membership: an element of a list, or a key of a map.
- **`c ? a : b`** evaluates only the branch it takes.

## Names and fields

- A **variable** is declared by your program, with its type. Its name
  may be qualified: a variable declared as `pane.id` is what `pane.id`
  reads, even where `pane` is a variable too. A leading dot, `.pane.id`,
  names it from the root.
- **`a.f`** is a struct's field, or a map's value at the key `'f'`.
  **`a[k]`** is a list's element (at an `int`) or a map's value. A missing
  key or an index out of range is an error when the expression runs.
- **`has(a.f)`** is whether the map `a` holds the key `'f'`.
- The words CEL reserves (`in`, `if`, `null`, `true`, `package`, …) can't
  name a variable or a helper, but a field may have one: `x.if`.
  `cellite.ReservedWords()` lists them.
- A variable may take the name of a type (`list`, `int`, `type`), and the
  name then reads the variable. Without such a variable, a type's name in
  an expression is a type value, which cel-lite refuses.

## Functions

| Function                                  | Meaning                                        |
| ----------------------------------------- | ---------------------------------------------- |
| `size(x)`, `x.size()`                     | a string's code points, bytes' length, a list's or a map's entries |
| `s.contains(t)`                           | whether `s` holds `t`                          |
| `s.startsWith(t)`, `s.endsWith(t)`        | whether `s` begins, or ends, with `t`          |
| `int(x)`                                  | from a uint, a double (toward zero) or a string |
| `uint(x)`                                 | from an int, a double or a string              |
| `double(x)`                               | from an int, a uint or a string                |
| `string(x)`                               | from a number, a bool, or bytes that are UTF-8 |
| `bytes(x)`                                | a string's UTF-8                               |
| `bool(x)`                                 | from `'true'`, `'TRUE'`, `'True'`, `'t'`, `'1'`, and the same for false |
| `dyn(x)`                                  | `x`, of type dyn                               |

A conversion that doesn't fit, `int(1e19)` or `int('ten')`, is an error
when the expression runs. So is one whose argument isn't of the type
Check gave it, as untyped data can give a variable: `int(s)` with `s` a
string variable given an int, as in cel-go. Your program can add functions of its own:
[helpers](embedding.md#helpers).

## Errors and unknowns

An expression that compiles can still fail as it runs: a division by
zero, an overflow, a missing key, a variable the activation doesn't have.
A variable can also be **unknown**: its value isn't there yet, as in
partial evaluation.

An error or an unknown is the result of whatever depends on it, except
that `&&` and `||` look at both sides, in either order:

- `false && x` and `x && false` are `false`, and `true || x` and
  `x || true` are `true`, whatever `x` is: an error, an unknown, or a
  value.
- When neither side decides, an unknown wins over an error: with `x`
  unknown, `x && 1 / 0 == 1` is unknown.
- `c ? a : b` is unknown or an error when `c` is.

## What cel-lite refuses

Each of these is refused when the expression compiles, with an
`*cellite.Error` whose `Kind` is `Unsupported` and whose `Name` names the
feature:

| Feature                     | Example                         | `Error.Name`               |
| --------------------------- | ------------------------------- | -------------------------- |
| Comprehension macros        | `tags.exists(t, t == 'x')`      | `macro exists`, `macro all`, … |
| Regular expressions         | `name.matches('^a')`            | `function matches`         |
| Timestamps and durations    | `timestamp('2026-01-01T00:00:00Z')` | `function timestamp`, `function duration`, `function getHours`, … |
| Type values                 | `type(x) == int`                | `function type`, `type int` |
| Message construction        | `Item{title: 'a'}`              | `message construction`     |
| Optional syntax             | `a.?b`, `[?x]`, `{?'k': v}`     | `optional syntax`          |
| Escaped identifiers         | ``a.`b-c` ``                    | `escaped identifier`       |
| `has()` on a struct's field | `has(item.title)`               | `has() on a struct`        |
| Comparing structs (not with `null`) | `item == other`         | `== on structs`            |
| Map keys of other types     | `{1.5: 'a'}`                    | `map key double`           |
| Inferred types              | `-x['k']` with `x` dyn          | `inferred type`            |

The macros are all, exists, exists_one, map and filter. The last row is
explained [below](#where-cel-lite-and-cel-go-differ).

A dyn can hide a struct from the checker: `x == y` with both dyn, or
`has(x.f)`. When the values turn out to be structs, the refusal comes as
the expression runs, as an `*EvalError` that says it is not supported,
unless the answer doesn't depend on it: `[x, 1] == [y, 2]` is `false`,
and `'a' in [x, 'a']` is `true`.

## Where cel-lite and cel-go differ

Where cel-go strays from the language definition, cel-lite follows the
definition, or refuses by name. The comparison with cel-go leaves out each
of these cases by name.

- **Inferred types.** cel-go types a value read with `[]` from a dyn
  (`x['k']`, or an element of an empty literal) with a type parameter that
  its first use binds: `-x['k']` is a double there, `[x['k'], 'a']` a
  `list(string)`, `-x['k'] + 1` is refused, and `int(x['k'])` takes only
  an int when it runs. In the definition the value is dyn. cel-lite
  refuses such a use (`inferred type`); `x.k` or `dyn(x['k'])` keeps the
  value dyn.
- **Map keys.** A map's keys are bool, int, uint or string, as the
  definition says. A map literal whose dyn key turns out a double, a list
  or a map is an error when it runs; cel-go builds the map, where a NaN
  key is never found and two NaN keys are two entries.
- **A variable named after a type, absent.** When a variable named `list`
  or `int` is declared but the activation doesn't have it, cel-go reads
  the type instead, a value the expression's checked type doesn't allow:
  `list` declared a bool gives the type `list`. cel-lite reports the
  variable missing, as for any other name.
- **`[]` on a struct.** cel-go reads a struct's field by its name with
  `[]` when a dyn hides the struct: `dyn(item)['title']`. In the
  definition `[]` reads a list or a map, and a field is read with `.`;
  cel-lite gives an error (no such overload).
- **A bytes key.** `x in m` with `x` bytes, as untyped data can give a
  variable declared a string, is `false` on any map: bytes are never a
  key. cel-go fails on a map literal, which can't hash them, and says
  `false` on a Go map.
- **Limits.** cel-lite reads shorter and shallower expressions than
  cel-go: see [Limits](tinygo.md#limits).

Error messages are cel-lite's own. Type errors read as cel-go's do
(`found no matching overload for '_+_' applied to '(string, int)'`), and
so do undeclared names; syntax errors are worded differently
(`syntax error: unexpected end of input`).
