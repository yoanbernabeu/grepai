package trace

// protobufQueries captures messages, services, RPC methods, and enums.
// The grammar wraps each definition's name in a dedicated node type
// (message_name, service_name, rpc_name, enum_name) so the queries are
// straightforward.
var protobufQueries = []NamedQuery{
	{Kind: "message", Query: `(message (message_name) @name)`},
	{Kind: "service", Query: `(service (service_name) @name)`},
	{Kind: "rpc", Query: `(rpc (rpc_name) @name)`},
	{Kind: "enum", Query: `(enum (enum_name) @name)`},
}
