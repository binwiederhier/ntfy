<?php

// Check out https://ntfy.sh/phil_alerts in your browser after running this.
file_get_contents('https://ntfy.sh/phil_alerts', false, stream_context_create([
    'http' => [
        'method' => 'POST', // PUT also works
        'header' =>
            "Content-Type: text/plain\r\n" .
            "Title: Unauthorized access detected\r\n" .
            "Priority: urgent\r\n" .
            "Tags: warning,skull",
        'content' => 'Remote access to phils-laptop detected. Act right away.'
    ]
]));
