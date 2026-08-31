package foo

class Greeter {
    fun hello() { }
}

interface Greet { fun hello() }

object Singleton { fun work() = 42 }

fun standalone() = 1

enum class Color { RED, GREEN, BLUE }
