module Kernel
  def halt(msg="")
    puts msg
    exit 1
  end

  def rows_expected!(actual, expected)
    if actual != expected
      puts "Expected #{expected} rows, got #{actual}"
      raise Sequel::Rollback
    end
  end
end
