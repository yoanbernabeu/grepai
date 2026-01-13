package trace

import (
	"context"
	"testing"
)

func TestRegexExtractor_SupportedLanguages(t *testing.T) {
	extractor := NewRegexExtractor()
	langs := extractor.SupportedLanguages()

	expected := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".jsx":  true,
		".tsx":  true,
		".py":   true,
		".php":  true,
		".c":    true,
		".h":    true,
		".zig":  true,
		".rs":   true,
		".cpp":  true,
		".hpp":  true,
		".cc":   true,
		".cxx":  true,
		".hxx":  true,
		".java": true,
		".cs":   true,
	}

	for _, lang := range langs {
		if !expected[lang] {
			t.Errorf("unexpected language extension: %s", lang)
		}
		delete(expected, lang)
	}

	for lang := range expected {
		t.Errorf("missing language extension: %s", lang)
	}
}

func TestRegexExtractor_ExtractSymbols_C(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `#include <stdio.h>

typedef struct {
    int x;
    int y;
} Point;

struct Rectangle {
    int width;
    int height;
};

int calculate_area(int width, int height) {
    return width * height;
}

void print_result(int value) {
    printf("%d\n", value);
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.c", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundFunctions := make(map[string]bool)
	foundTypes := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindFunction:
			foundFunctions[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		}
	}

	expectedFunctions := []string{"calculate_area", "print_result"}
	for _, name := range expectedFunctions {
		if !foundFunctions[name] {
			t.Errorf("missing function: %s", name)
		}
	}

	expectedTypes := []string{"Point", "Rectangle"}
	for _, name := range expectedTypes {
		if !foundTypes[name] {
			t.Errorf("missing type: %s", name)
		}
	}
}

func TestRegexExtractor_ExtractSymbols_Zig(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	// Test code based on real Zig standard library patterns
	content := `const std = @import("std");

pub const Point = struct {
    x: i32,
    y: i32,

    pub fn init(x: i32, y: i32) Point {
        return Point{ .x = x, .y = y };
    }

    pub inline fn distance(self: Point, other: Point) i32 {
        return self.x - other.x;
    }
};

pub const Color = enum {
    red,
    green,
    blue,

    pub fn isRed(self: Color) bool {
        return self == .red;
    }
};

pub const Alignment = enum(u8) {
    @"1" = 0,
    @"2" = 1,

    pub fn toByteUnits(a: Alignment) usize {
        return @as(usize, 1) << @intFromEnum(a);
    }

    pub const Mode = enum {
        decimal,
        binary,
    };
};

fn calculate_area(width: i32, height: i32) i32 {
    return width * height;
}

pub inline fn helper() void {}

export fn exported_func() void {}

pub fn main() void {
    const area = calculate_area(10, 20);
    std.debug.print("{}\n", .{area});
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.zig", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundFunctions := make(map[string]bool)
	foundMethods := make(map[string]bool)
	foundTypes := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindFunction:
			foundFunctions[sym.Name] = true
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		}
	}

	// Top-level functions
	expectedFunctions := []string{"calculate_area", "main", "helper", "exported_func"}
	for _, name := range expectedFunctions {
		if !foundFunctions[name] {
			t.Errorf("missing function: %s", name)
		}
	}

	// Methods inside structs/enums
	expectedMethods := []string{"init", "distance", "isRed", "toByteUnits"}
	for _, name := range expectedMethods {
		if !foundMethods[name] {
			t.Errorf("missing method: %s", name)
		}
	}

	// Types (structs, enums)
	expectedTypes := []string{"Point", "Color", "Alignment", "Mode"}
	for _, name := range expectedTypes {
		if !foundTypes[name] {
			t.Errorf("missing type: %s", name)
		}
	}
}

func TestRegexExtractor_ExtractSymbols_Rust(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `struct Point {
    x: i32,
    y: i32,
}

enum Color {
    Red,
    Green,
    Blue,
}

trait Drawable {
    fn draw(&self);
}

fn calculate_area(width: i32, height: i32) -> i32 {
    width * height
}

pub fn main() {
    let area = calculate_area(10, 20);
    println!("{}", area);
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.rs", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundFunctions := make(map[string]bool)
	foundTypes := make(map[string]bool)
	foundTraits := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindFunction:
			foundFunctions[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		case KindInterface:
			foundTraits[sym.Name] = true
		}
	}

	expectedFunctions := []string{"calculate_area", "main"}
	for _, name := range expectedFunctions {
		if !foundFunctions[name] {
			t.Errorf("missing function: %s", name)
		}
	}

	expectedTypes := []string{"Point", "Color"}
	for _, name := range expectedTypes {
		if !foundTypes[name] {
			t.Errorf("missing type: %s", name)
		}
	}

	if !foundTraits["Drawable"] {
		t.Error("missing trait: Drawable")
	}
}

func TestRegexExtractor_ExtractSymbols_Cpp(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `#include <iostream>
#include <vector>

template<typename T>
class Container {
public:
    void push(T value) {
        data_.push_back(value);
    }

    T pop() {
        T val = data_.back();
        data_.pop_back();
        return val;
    }

    size_t size() const {
        return data_.size();
    }

private:
    std::vector<T> data_;
};

class Point {
public:
    int x;
    int y;

    int distance(const Point& other) const {
        return abs(x - other.x) + abs(y - other.y);
    }
};

struct Rectangle {
    int width;
    int height;
};

enum class Color {
    Red,
    Green,
    Blue
};

int calculate_area(int width, int height) {
    return width * height;
}

void print_result(int value) {
    std::cout << value << std::endl;
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "test.cpp", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundFunctions := make(map[string]bool)
	foundMethods := make(map[string]bool)
	foundClasses := make(map[string]bool)
	foundTypes := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindFunction:
			foundFunctions[sym.Name] = true
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindClass:
			foundClasses[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		}
	}

	expectedFunctions := []string{"calculate_area", "print_result"}
	for _, name := range expectedFunctions {
		if !foundFunctions[name] {
			t.Errorf("missing function: %s", name)
		}
	}

	// Methods inside classes
	expectedMethods := []string{"push", "pop", "size", "distance"}
	for _, name := range expectedMethods {
		if !foundMethods[name] {
			t.Errorf("missing method: %s", name)
		}
	}

	expectedClasses := []string{"Container", "Point", "Rectangle"}
	for _, name := range expectedClasses {
		if !foundClasses[name] {
			t.Errorf("missing class: %s", name)
		}
	}

	if !foundTypes["Color"] {
		t.Error("missing enum type: Color")
	}
}

func TestRegexExtractor_ExtractSymbols_Java(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `package com.example.app;

import java.util.List;
import java.util.Map;
import java.io.IOException;

/**
 * Main application class demonstrating various Java constructs.
 */
public class Application extends BaseApp implements Runnable {

    private String name;
    private static int counter = 0;

    // Constructor
    public Application(String name) {
        this.name = name;
    }

    // Static method
    public static Application create(String name) {
        return new Application(name);
    }

    // Instance method with generics
    public List<String> getItems(Map<String, Integer> filter) {
        return null;
    }

    // Synchronized method
    public synchronized void process() {
        counter++;
    }

    // Method with throws clause
    public void load() throws IOException {
        // implementation
    }

    // Final method
    public final void complete() {
        System.out.println("Complete");
    }

    // Override from Runnable
    @Override
    public void run() {
        process();
    }

    // Inner class
    private static class Helper {
        public void assist() {
            // helper method
        }
    }

    // Inner enum
    public enum Status {
        PENDING,
        ACTIVE,
        COMPLETED;

        public boolean isActive() {
            return this == ACTIVE;
        }
    }
}

// Interface with generics
public interface Repository<T> {

    T findById(Long id);

    List<T> findAll();

    default void log(String message) {
        System.out.println(message);
    }
}

// Abstract class
public abstract class BaseService {

    public abstract void execute();

    protected void init() {
        // initialization
    }
}

// Annotation type
public @interface Deprecated {
}

// Enum at top level
public enum Priority {
    LOW,
    MEDIUM,
    HIGH
}

// Record (Java 14+)
public record Point(int x, int y) {

    public int distanceFromOrigin() {
        return (int) Math.sqrt(x * x + y * y);
    }
}

// Generic class
public class Container<T> {

    private T value;

    public T getValue() {
        return value;
    }

    public void setValue(T value) {
        this.value = value;
    }
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "Application.java", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundMethods := make(map[string]bool)
	foundClasses := make(map[string]bool)
	foundInterfaces := make(map[string]bool)
	foundTypes := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindClass:
			foundClasses[sym.Name] = true
		case KindInterface:
			foundInterfaces[sym.Name] = true
		case KindType:
			foundTypes[sym.Name] = true
		}
	}

	// Test methods (including constructors)
	expectedMethods := []string{
		"Application",        // constructor
		"create",             // static method
		"getItems",           // method with generics
		"process",            // synchronized method
		"load",               // method with throws
		"complete",           // final method
		"run",                // override method
		"assist",             // inner class method
		"isActive",           // enum method
		"findById",           // interface method
		"findAll",            // interface method
		"log",                // default method
		"execute",            // abstract method
		"init",               // protected method
		"distanceFromOrigin", // record method
		"getValue",           // generic class method
		"setValue",           // generic class method
	}
	for _, name := range expectedMethods {
		if !foundMethods[name] {
			t.Errorf("missing method: %s", name)
		}
	}

	// Test classes (including enums and records)
	expectedClasses := []string{
		"Application", // main class
		"Helper",      // inner class
		"BaseService", // abstract class
		"Priority",    // top-level enum
		"Point",       // record
		"Container",   // generic class
	}
	for _, name := range expectedClasses {
		if !foundClasses[name] {
			t.Errorf("missing class: %s", name)
		}
	}

	// Test interfaces (including annotations)
	expectedInterfaces := []string{
		"Repository", // generic interface
		"Deprecated", // annotation type
	}
	for _, name := range expectedInterfaces {
		if !foundInterfaces[name] {
			t.Errorf("missing interface: %s", name)
		}
	}

	// Test inner enum as type
	if !foundTypes["Status"] {
		t.Error("missing inner enum type: Status")
	}
}

func TestRegexExtractor_ExtractSymbols_CSharp(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `using System;
using System.Collections.Generic;

namespace Sample.App;

public interface IRepository<T>
{
    T GetById(Guid id);
    IEnumerable<T> GetAll();
}

public record User(Guid Id, string Name)
{
    public string DisplayName() => $"{Name}";
}

public abstract class BaseService
{
    protected abstract void Execute();
}

public class UserService : BaseService
{
    private readonly IRepository<User> _repo;

    public UserService(IRepository<User> repo)
    {
        _repo = repo;
    }

    public override void Execute()
    {
        var user = new User(Guid.NewGuid(), "Ada");
        Console.WriteLine(user.DisplayName());
        _repo.GetById(user.Id);
    }

    public IEnumerable<User> GetUsers() => _repo.GetAll();
}

public readonly struct Result<T>
{
    public T Value { get; }
    public Result(T value) => Value = value;
}
`

	symbols, err := extractor.ExtractSymbols(ctx, "UserService.cs", content)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}

	foundMethods := make(map[string]bool)
	foundClasses := make(map[string]bool)
	foundInterfaces := make(map[string]bool)

	for _, sym := range symbols {
		switch sym.Kind {
		case KindMethod:
			foundMethods[sym.Name] = true
		case KindClass:
			foundClasses[sym.Name] = true
		case KindInterface:
			foundInterfaces[sym.Name] = true
		}
	}

	expectedMethods := []string{
		"GetById",
		"GetAll",
		"DisplayName",
		"UserService",
		"Execute",
		"GetUsers",
		"Result",
	}
	for _, name := range expectedMethods {
		if !foundMethods[name] {
			t.Errorf("missing method: %s", name)
		}
	}

	expectedClasses := []string{"User", "BaseService", "UserService", "Result"}
	for _, name := range expectedClasses {
		if !foundClasses[name] {
			t.Errorf("missing class: %s", name)
		}
	}

	if !foundInterfaces["IRepository"] {
		t.Error("missing interface: IRepository")
	}
}

func TestRegexExtractor_ExtractReferences(t *testing.T) {
	extractor := NewRegexExtractor()
	ctx := context.Background()

	content := `fn helper() i32 {
    return 42;
}

pub fn main() void {
    const result = helper();
    std.debug.print("{}\n", .{result});
}
`

	refs, err := extractor.ExtractReferences(ctx, "test.zig", content)
	if err != nil {
		t.Fatalf("ExtractReferences failed: %v", err)
	}

	foundRefs := make(map[string]bool)
	for _, ref := range refs {
		foundRefs[ref.SymbolName] = true
	}

	if !foundRefs["helper"] {
		t.Error("missing reference to helper")
	}
}

func TestIsKeyword(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		expected bool
	}{
		{"if", "c", true},
		{"malloc", "c", true},
		{"myFunc", "c", false},
		{"if", "zig", true},
		{"comptime", "zig", true},
		{"myFunc", "zig", false},
		{"if", "rust", true},
		{"match", "rust", true},
		{"myFunc", "rust", false},
		{"if", "cpp", true},
		{"static_cast", "cpp", true},
		{"myFunc", "cpp", false},
		{"if", "java", true},
		{"new", "java", true},
		{"instanceof", "java", true},
		{"println", "java", true},
		{"myMethod", "java", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.lang, func(t *testing.T) {
			result := IsKeyword(tt.name, tt.lang)
			if result != tt.expected {
				t.Errorf("IsKeyword(%q, %q) = %v, want %v", tt.name, tt.lang, result, tt.expected)
			}
		})
	}
}
