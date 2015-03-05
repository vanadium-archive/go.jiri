// Tests the case where modules are used by a package but the internal
// test package does not include any tests, rather all of the tests are in
// an external test package and hence an appropriate TestMain needs to be
// be generated there.
package transitive_external
