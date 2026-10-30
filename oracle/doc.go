// Package oracle checks cellite against cel-go, which it holds as the
// oracle: the same expressions, compiled in the same declarations and run
// on the same values, must give the same types, values, errors and
// unknowns, or be refused by cellite by name. It is a module of its own so
// that cel-go is a requirement of these tests only, and never of cellite.
package oracle
