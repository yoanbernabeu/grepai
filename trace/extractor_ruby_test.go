package trace

import (
	"context"
	"testing"
)

func TestRubySymbolsAndCalls(t *testing.T) {
	content := `module Billing
  class Invoice < Base
    def self.build(attrs)
      new(attrs).tap(&:validate!)
    end

    # don't charge twice
    def paid?
      payments.any?
    end

    def total=(value)
      @total = compute_total(value)
    end
  end
end

def helper(x)
  Billing::Invoice.build(x)
end
`
	ext := NewRegexExtractor()
	symbols, refs, err := ext.ExtractAll(context.Background(), "app/models/invoice.rb", content)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]SymbolKind{}
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}
	for _, want := range []string{"Billing", "Invoice", "build", "paid?", "total=", "helper"} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing symbol %s; got %v", want, names)
		}
	}
	calls := map[string]bool{}
	for _, r := range refs {
		calls[r.SymbolName] = true
	}
	for _, want := range []string{"compute_total", "build", "any?", "tap"} {
		if !calls[want] {
			t.Errorf("missing call %s; got %v", want, calls)
		}
	}
	if calls["charge"] {
		t.Errorf("text in a comment was extracted as a call")
	}
}
