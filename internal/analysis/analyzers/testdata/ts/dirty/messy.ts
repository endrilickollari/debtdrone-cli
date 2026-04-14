function complexLogic(input: number): string {
    if (input > 10) {
        if (input < 20) {
            return "Medium";
        } else if (input < 50) {
            return "High";
        } else {
             if (input % 2 === 0) {
                 return "Very High Even";
             } else {
                 return "Very High Odd";
             }
        }
    } else {
        switch(input) {
            case 1:
                return "One";
            case 2:
                return "Two";
            case 3:
                return "Three";
            default:
                return "Low";
        }
    }
}
