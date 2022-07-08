import * as jose from 'jose'

async function publish() {
    const jwe = await new jose.CompactEncrypt(new TextEncoder().encode('Secret message from JS!'))
        .setProtectedHeader({ alg: 'dir', enc: 'A256GCM' })
        .encrypt(publicKey)
}
