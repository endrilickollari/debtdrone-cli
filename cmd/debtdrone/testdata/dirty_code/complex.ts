function complexFunction(x: number): void {
    if (x > 0) {
        for (let i = 0; i < x; i++) {
            if (i % 2 === 0) {
                if (i % 3 === 0) {
                    for (let j = 0; j < i; j++) {
                        if (j > 5) {
                            console.log("Deeply nested");
                        }
                    }
                }
            }
        }
    } else {
        if (x < -5) {
            console.log("Negative");
        }
    }
}
