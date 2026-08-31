package trace

import (
	"context"
	"testing"
)

func TestTreeSitterExtractor_ExtractSymbols_CSharp(t *testing.T) {
	extractor, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor failed: %v", err)
	}

	ctx := context.Background()
	content := `namespace Demo;

public interface IFoo {
    void DoWork();
}

public record Person(string Name);

public struct Point {
    public int X;
    public int Y;

    public Point(int x, int y) {
        X = x;
        Y = y;
    }

    public int Distance() {
        return X + Y;
    }
}

public class Greeter {
    public Greeter() {}

    public void SayHello() {
        Helper();
    }

    private void Helper() {}
}

public static class Util {
    public static void Run() {
        var greeter = new Greeter();
        greeter.SayHello();
    }
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.cs", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundMethods := make(map[string]bool)
	foundTypes := make(map[string]bool)
	foundInterfaces := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindClass:
			foundTypes[sym.Name] = true
		case KindInterface:
			foundInterfaces[sym.Name] = true
		}
	}

	for _, name := range []string{"Greeter", "Point", "Person", "Util"} {
		if !foundTypes[name] {
			t.Errorf("missing type: %s", name)
		}
	}

	if !foundInterfaces["IFoo"] {
		t.Error("missing interface: IFoo")
	}

	for _, name := range []string{"Greeter", "Point", "SayHello", "Helper", "Distance", "Run"} {
		if !foundMethods[name] {
			t.Errorf("missing method/constructor: %s", name)
		}
	}
}

func TestTreeSitterExtractor_ExtractReferences_CSharp(t *testing.T) {
	extractor, err := NewTreeSitterExtractor()
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor failed: %v", err)
	}

	ctx := context.Background()
	content := `public class Greeter {
    public void SayHello() {
        Helper();
    }

    private void Helper() {}

    public void CallHelper() {
        Helper();
    }
}
`

	refs, err := extractor.ExtractReferences(ctx, "test.cs", content)
	if err != nil {
		t.Fatalf("ExtractReferences failed: %v", err)
	}

	callers := make(map[string]string)
	for _, ref := range refs {
		if ref.SymbolName == "Helper" {
			callers[ref.CallerName] = ref.SymbolName
		}
	}

	if callers["SayHello"] != "Helper" {
		t.Error("expected Helper reference attributed to SayHello")
	}
	if callers["CallHelper"] != "Helper" {
		t.Error("expected Helper reference attributed to CallHelper")
	}
}
