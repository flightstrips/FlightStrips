// Package aman contains the provider-neutral core domain contract for the
// Arrival Manager. It deliberately has no dependency on persistence, source
// adapters, transport, presentation, or navigation implementations. Owning
// wire packages use its serialization helpers; its domain structs are not wire
// DTOs.
package aman
