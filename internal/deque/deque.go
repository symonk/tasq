package deque

// TODO: This is a stub implementation and garbage built on a slice for now
// just to have the core implementation working, we will swap it out later
// for something o(1) in pushleft/popright etc

type DoubleEndedQueue []func()
