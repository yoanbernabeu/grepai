package trace

// hclQueries captures top-level blocks (resource / variable / locals /
// data / etc.) by their identifier. The full set of HCL's per-block
// labels (e.g. resource "aws_instance" "web") are not captured here —
// they're string literals deeper in the block, and Kind="block" with
// Name=<block-type> is the useful navigation hook for Terraform/Packer.
var hclQueries = []NamedQuery{
	{Kind: "block", Query: `(block (identifier) @name)`},
}
