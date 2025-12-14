public class Complex {
    public void complexMethod(int x) {
        if (x > 0) {
            for (int i = 0; i < x; i++) {
                if (i % 2 == 0) {
                    if (i % 3 == 0) {
                        for (int j = 0; j < i; j++) {
                            try {
                                if (j > 5) {
                                    System.out.println("Deeply nested");
                                }
                            } catch (Exception e) {
                                e.printStackTrace();
                            }
                        }
                    }
                }
            }
        }
    }
}
