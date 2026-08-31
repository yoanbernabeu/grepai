package foo;

public class Greeter implements Greet {
    public static final int MAX_USERS = 100;
    private String name;
    private final int id = 1;

    public Greeter(String name) {}
    public void hello() {}
    private int compute(int x) { return x * 2; }
}

interface Greet { void hello(); }

enum Color { RED, GREEN, BLUE }

record Point(int x, int y) {}
