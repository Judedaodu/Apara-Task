package com.apara.core

import org.springframework.beans.factory.annotation.Value
import org.springframework.stereotype.Service
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ThreadLocalRandom
import java.util.concurrent.atomic.AtomicLong
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

@Service
class SettlementService(
    @Value("\${SPONSOR_HMAC_SECRET:}") private val sponsorSecret: String,
    @Value("\${AKN_POOL_INITIAL:10000000}") aknInitial: Long,
    @Value("\${PAW_POOL_INITIAL:500000}") pawInitial: Long,
    @Value("\${CORDAPP_FAILURE_RATE:0}") private val cordappFailureRate: Double,
    @Value("\${SPONSOR_DELAY_SECONDS:60}") private val sponsorDelaySeconds: Long,
) {
    private val seenFingerprints: MutableSet<String> = ConcurrentHashMap.newKeySet()
    private val aknPool = AtomicLong(aknInitial)
    private val pawPool = AtomicLong(pawInitial)

    fun verifySponsorSignature(rawBody: ByteArray, signatureHex: String?): Boolean {
        if (sponsorSecret.isBlank()) {
            return true
        }
        if (signatureHex.isNullOrBlank()) {
            return false
        }
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(sponsorSecret.toByteArray(Charsets.UTF_8), "HmacSHA256"))
        val expected = mac.doFinal(rawBody).joinToString("") { b -> "%02x".format(b) }
        return constantTimeEquals(expected, signatureHex.lowercase())
    }

    fun process(req: ReceiveSubmitRequest): ReceiveSubmitResponse {
        val now = System.currentTimeMillis()
        if (!seenFingerprints.add(req.fingerprint)) {
            return ReceiveSubmitResponse(
                fingerprint = req.fingerprint,
                state = "GHOST_DETECTED",
                timestamp = now,
                message = "duplicate fingerprint (double-spend attempt)",
            )
        }

        if (cordappFailureRate > 0 && ThreadLocalRandom.current().nextDouble() < cordappFailureRate) {
            seenFingerprints.remove(req.fingerprint)
            return ReceiveSubmitResponse(
                fingerprint = req.fingerprint,
                state = "FAILED",
                timestamp = now,
                message = "simulated Corda / Cordapp failure",
            )
        }

        if (req.currency.equals("AKN", ignoreCase = true)) {
            var settled = false
            aknPool.updateAndGet { current ->
                val next = current - req.amount
                if (next < 0) {
                    current
                } else {
                    settled = true
                    next
                }
            }
            if (!settled) {
                seenFingerprints.remove(req.fingerprint)
                return ReceiveSubmitResponse(
                    fingerprint = req.fingerprint,
                    state = "POOL_REJECTED",
                    timestamp = now,
                    message = "AKN pool would go negative; state unchanged",
                )
            }
            pawPool.addAndGet(req.amount)
        }

        val state = if (isDelayedScenario(req)) {
            "DELAYED"
        } else {
            "SETTLED"
        }

        return ReceiveSubmitResponse(
            fingerprint = req.fingerprint,
            state = state,
            timestamp = now,
            message = if (state == "DELAYED") {
                "sponsor delay active (SPONSOR_DELAY_SECONDS=$sponsorDelaySeconds); settlement queued"
            } else {
                null
            },
        )
    }

    /** Deterministic “delayed sponsor” path for demos: amount divisible by 1_000_000 and delay env &gt; 0. */
    private fun isDelayedScenario(req: ReceiveSubmitRequest): Boolean {
        return sponsorDelaySeconds > 0 && req.amount > 0 && req.amount % 1_000_000L == 0L
    }

    private fun constantTimeEquals(a: String, b: String): Boolean {
        if (a.length != b.length) return false
        var diff = 0
        for (i in a.indices) {
            diff = diff or (a[i].code xor b[i].code)
        }
        return diff == 0
    }
}
