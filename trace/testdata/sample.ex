defmodule Greeter do
  def hello(name), do: "hi #{name}"

  defp helper(x) do
    x * 2
  end

  defmacro greet(name), do: quote do: hello(unquote(name))
end

defprotocol Greet do
  def hello(thing)
end

defmodule Multi.Nested do
  def a, do: 1
  def b, do: 2
end
