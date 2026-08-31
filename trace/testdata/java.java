package foo;

public class Greeter implements Greet {
    public Greeter(String name) {}
    public void hello() {}
    private int compute(int x) { return x * 2; }
}

interface Greet { void hello(); }

enum Color { RED, GREEN, BLUE }

record Point(int x, int y) {}
