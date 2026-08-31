module Foo exposing (..)

type alias Point = { x : Int, y : Int }

type Color = Red | Green | Blue

greet : String -> String
greet name = "hi " ++ name
