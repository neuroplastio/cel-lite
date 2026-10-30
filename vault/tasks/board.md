# cel-lite board

Top first. Claim an item before working on it; drop it when done.

- A Value of 32 bytes. Half of eval's time under gc is copying the
  48-byte Value each call returns: gc keeps no struct over 32 bytes in
  registers, and the copy's loads stall on the stores that spilled it.
  At 32 bytes (kind, a number, and a string's or an interface's two
  words in one place) the copies go. It needs unsafe for the union and
  rests on gc's and TinyGo's interface layout, and ObjectValue's struct
  name needs a home: a decision before the work. Estimate: eval up to
  twice as fast under gc, less under TinyGo. Not now (gov, 2026-10-30):
  eval is well within its users' needs, and unsafe that rests on two
  runtimes' layouts is a risk for a module meant for other runtimes. It
  comes back if a user measures eval as their cost.
- A tri-state Test path for bool contexts (true, false, unknown, without
  a Value), which skips the Value copies for keymaps. Not needed while
  the 32-byte Value is off (gov, 2026-10-30); the 32-byte Value would
  make it unnecessary.
- Values of another kind than declared, as untyped data gives them. The
  oracle binds each variable to its own type only. With a string
  variable given an int, a conversion converts by the value's kind
  (`int(s)` is 1) where cel-go keeps the overload Check chose and errors;
  `s in {'a': 'x'}` with s bytes is false where cel-go errors. Every
  other operator tried errors as cel-go does. Check a conversion's
  argument against its checked overload, and add an oracle test that
  binds variables to values of other kinds. For v0.1.1.
- `x == null` where x is a struct: Check refuses it as comparing
  structs, but it compares none, and cel-go allows it (with a native
  type's nil pointer read as a zero struct, it is false there). Allow
  it, with oracle cases in TestOracleStructs.
- Exported CEL-exact helpers for hotty-surf's compiled mode: numeric ==
  and comparisons across kinds, overflow-checked + - *, / and % by zero,
  the dyn paths.
