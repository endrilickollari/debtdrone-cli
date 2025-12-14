def complex_function(x)
  if x > 0
    x.times do |i|
      if i % 2 == 0
        if i % 3 == 0
          i.times do |j|
            if j > 5
              puts "Deeply nested"
            end
          end
        end
      end
    end
  else
    if x < -5
      puts "Negative"
    end
  end
end
