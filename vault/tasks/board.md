# cel-lite board

Top first. Claim an item before working on it; drop it when done.

- A Value of 32 bytes. Half of eval's time under gc is copying the
  48-byte Value each call returns: gc keeps no struct over 32 bytes in
  registers, and the copy's loads stall on the stores that spilled it.
  At 32 bytes (kind, a number, and a string's or an interface's two
  words in one place) the copies go. It needs unsafe for the union and
  rests on gc's and TinyGo's interface layout, and ObjectValue's struct
  name needs a home: a decision before the work. Estimate: eval up to
  twice as fast under gc, less under TinyGo.
- Structs in the oracle (cel-go's native types), beside the unit tests.
- Exported CEL-exact helpers for hotty-surf's compiled mode: numeric ==
  and comparisons across kinds, overflow-checked + - *, / and % by zero,
  the dyn paths.
