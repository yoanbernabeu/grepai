package foo

class Greeter {
    val name: String = "alice"
    var counter: Int = 0
    const val MAX: Int = 100

    fun hello() { }
}

interface Greet { fun hello() }

object Singleton {
    val region = "us-east-1"
    fun work() = 42
}

fun standalone() = 1

enum class Color { RED, GREEN, BLUE }
