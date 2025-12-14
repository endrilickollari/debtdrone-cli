def complex_function(x):
    if x > 0:
        for i in range(x):
            if i % 2 == 0:
                if i % 3 == 0:
                    for j in range(i):
                        if j > 5:
                            print("Deeply nested")
                        elif j < 0:
                            print("Impossible")
                    else:
                        print("Loop finished")
    else:
        if x < -5:
            try:
                print("Negative")
            except Exception:
                pass
