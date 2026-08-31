package foo

object Greeter {
  def hello(name: String): Unit = println(s"hi $name")
  val MAX = 100
}

class Hello(name: String) {
  def say(): String = s"hi $name"
}

trait Greet { def hello(): Unit }

case class Point(x: Int, y: Int)

enum Color { case Red, Green, Blue }
