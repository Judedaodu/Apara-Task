package com.apara.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SettlementServiceTest {

    @Test
    fun `ghost payment detected`() {
        val svc = SettlementService("", 10_000_000, 500_000, 0.0, 0)
        val req = ReceiveSubmitRequest("fp1", "t", 100, "AKN", "bank")
        assertEquals("SETTLED", svc.process(req).state)
        assertEquals("GHOST_DETECTED", svc.process(req).state)
    }

    @Test
    fun `pool rejects without corrupting balance`() {
        val svc = SettlementService("", 1000, 500_000, 0.0, 0)
        val big = ReceiveSubmitRequest("fp2", "t", 2000, "AKN", "bank")
        assertEquals("POOL_REJECTED", svc.process(big).state)
        val ok = ReceiveSubmitRequest("fp3", "t", 500, "AKN", "bank")
        assertEquals("SETTLED", svc.process(ok).state)
    }

    @Test
    fun `hmac verification`() {
        val svc = SettlementService("secret", 10_000_000, 500_000, 0.0, 0)
        val body = """{"fingerprint":"x","templateId":"t","amount":1,"currency":"AKN","senderBank":"b"}""".toByteArray()
        val mac = javax.crypto.Mac.getInstance("HmacSHA256")
        mac.init(javax.crypto.spec.SecretKeySpec("secret".toByteArray(), "HmacSHA256"))
        val sig = mac.doFinal(body).joinToString("") { b -> "%02x".format(b) }
        assertTrue(svc.verifySponsorSignature(body, sig))
        assertTrue(!svc.verifySponsorSignature(body, "deadbeef"))
    }
}
