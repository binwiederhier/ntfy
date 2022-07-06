<?php

$message = [
    "message" => "Secret!",
    "priority" => 5
];
$plaintext = json_encode($message);
$key = deriveKey("secr3t password", "https://ntfy.sh/mysecret");
$ciphertext = encrypt($plaintext, $key);

file_get_contents('https://ntfy.sh/mysecret', false, stream_context_create([
    'http' => [
        'method' => 'POST', // PUT also works
        'header' =>
            "Content-Type: text/plain\r\n" .
            "Encryption: jwe",
        'content' => $ciphertext
    ]
]));

function deriveKey($password, $topicUrl)
{
    $salt = hex2bin(hash("sha256", $topicUrl));
    return openssl_pbkdf2($password, $salt, 32, 50000, "sha256");
}

function encrypt(string $plaintext, string $key): string
{
    $encodedHeader = base64url_encode(json_encode(["alg" => "dir", "enc" => "A256GCM"]));
    $iv = openssl_random_pseudo_bytes(12); // GCM is used with a 96-bit IV
    $aad = $encodedHeader;
    $tag = null;
    $content = openssl_encrypt($plaintext, "aes-256-gcm", $key, OPENSSL_RAW_DATA, $iv, $tag, $aad);
    return
        $encodedHeader . "." .
        "." . // No content encryption key (CEK) in "dir" mode
        base64url_encode($iv) . "." .
        base64url_encode($content) . "." .
        base64url_encode($tag);
}

function base64url_encode($input)
{
    return str_replace('=', '', strtr(base64_encode($input), '+/', '-_'));
}

