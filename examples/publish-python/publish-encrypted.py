#!/usr/bin/env python3

import requests

from base64 import b64encode, urlsafe_b64encode, b64decode
from Crypto.Cipher import AES
from Crypto.Protocol.KDF import PBKDF2
from Crypto.Hash import SHA256
from Crypto.Random import get_random_bytes


def derive_key(password, topic_url):
    salt = SHA256.new(data=topic_url.encode('utf-8')).digest()
    return PBKDF2(password, salt, 32, count=50000, hmac_hash_module=SHA256)


def encrypt(plaintext, key):
    encoded_header = b64urlencode('{"alg":"dir","enc":"A256GCM"}'.encode('utf-8'))
    iv = get_random_bytes(12)  # GCM is used with a 96-bit IV
    aad = encoded_header
    cipher = AES.new(key, AES.MODE_GCM, nonce=iv)
    cipher.update(aad.encode('utf-8'))
    ciphertext, tag = cipher.encrypt_and_digest(plaintext.encode('utf-8'))
    return "{header}..{iv}.{ciphertext}.{tag}".format(
        header=encoded_header,
        iv=b64urlencode(iv),
        ciphertext=b64urlencode(ciphertext),
        tag=b64urlencode(tag)
    )


def b64urlencode(b):
    return urlsafe_b64encode(b).decode('utf-8').replace("=", "")


key = derive_key("secr3t password", "https://ntfy.sh/mysecret")
ciphertext = encrypt('{"message":"Python says hi","tags":["secret"]}', key)

resp = requests.post("https://ntfy.sh/mysecret", data=ciphertext, headers={"Encryption": "jwe"})
resp.raise_for_status()
