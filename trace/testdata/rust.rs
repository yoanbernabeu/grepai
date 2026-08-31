pub fn add(x: i32, y: i32) -> i32 { x + y }

pub struct Point { x: f64, y: f64 }

impl Point {
    pub fn new(x: f64, y: f64) -> Self { Point { x, y } }
}

pub trait Greet { fn hello(&self); }

pub enum Color { Red, Green, Blue(u8) }

const MAX: u32 = 100;
static GLOBAL: i32 = 42;
type Pair = (i32, i32);
