<?php

$fp = fopen('https://ntfy.sh/phil_alerts/json', 'r');
if (!$fp) {
    die('cannot open stream');
}
while (!feof($fp)) {
    $buffer = fgets($fp, 2048);
    echo $buffer;
    flush();
}
fclose($fp);
