package com.apara.core

import jakarta.servlet.http.HttpServletRequest
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RestController

@RestController
class CoreController(private val settlementService: SettlementService) {

    @PostMapping("/core/receive-submit")
    fun receiveSubmit(
        @RequestBody body: ReceiveSubmitRequest,
        request: HttpServletRequest,
    ): ResponseEntity<ReceiveSubmitResponse> {
        val raw = request.getAttribute("cachedBody") as? ByteArray
            ?: return ResponseEntity.status(HttpStatus.BAD_REQUEST).build()
        val sig = request.getHeader("X-Sponsor-Signature")
        if (!settlementService.verifySponsorSignature(raw, sig)) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
                .body(
                    ReceiveSubmitResponse(
                        fingerprint = body.fingerprint,
                        state = "INVALID_SIGNATURE",
                        timestamp = System.currentTimeMillis(),
                        message = "HMAC verification failed",
                    ),
                )
        }
        val res = settlementService.process(body)
        val status = when (res.state) {
            "GHOST_DETECTED", "POOL_REJECTED" -> HttpStatus.CONFLICT
            "FAILED" -> HttpStatus.BAD_GATEWAY
            else -> HttpStatus.OK
        }
        return ResponseEntity.status(status).body(res)
    }
}
