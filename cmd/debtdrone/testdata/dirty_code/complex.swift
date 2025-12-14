func complexFunction(x: Int) {
    if x > 0 {
        for i in 0..<x {
            if i % 2 == 0 {
                if i % 3 == 0 {
                    for j in 0..<i {
                        if j > 5 {
                            print("Deeply nested")
                        }
                    }
                }
            }
        }
    } else {
        if x < -5 {
            print("Negative")
        }
    }
}
