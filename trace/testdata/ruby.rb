# Test fixture for Ruby symbol extraction. Symbol set is asserted by
# TestExtractor_Ruby in extractor_lang_test.go.

module Greeter
  class Hello
    def say(name)
      puts "hi #{name}"
    end

    def self.banner
      "!"
    end
  end
end

class Standalone
end
