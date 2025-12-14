using System;

class Complex {
    public void ComplexMethod(int x) {
        if (x > 0) {
            for (int i = 0; i < x; i++) {
                if (i % 2 == 0) {
                    if (i % 3 == 0) {
                        for (int j = 0; j < i; j++) {
                            if (j > 5) {
                                Console.WriteLine("Deeply nested");
                            }
                        }
                    }
                }
            }
        } else {
            if (x < -5) {
                Console.WriteLine("Negative");
            }
        }
    }
}
