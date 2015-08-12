module Kernel
  def halt(msg="")
    puts msg
    exit 1
  end

  def rows_expected!(actual, expected)
    puts "Expected #{expected} rows, got #{actual}" if actual != expected
    raise Sequel::Rollback
  end
end
