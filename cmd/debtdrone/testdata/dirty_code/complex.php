<?php
function complexFunction($x)
{
    if ($x > 0) {
        for ($i = 0; $i < $x; $i++) {
            if ($i % 2 == 0) {
                if ($i % 3 == 0) {
                    for ($j = 0; $j < $i; $j++) {
                        if ($j > 5) {
                            echo "Deeply nested";
                        }
                    }
                }
            }
        }
    } else {
        if ($x < -5) {
            echo "Negative";
        }
    }
}
