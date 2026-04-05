package com.apara.core

import com.fasterxml.jackson.annotation.JsonInclude

data class ReceiveSubmitRequest(
    val fingerprint: String,
    val templateId: String,
    val amount: Long,
    val currency: String,
    val senderBank: String,
)

@JsonInclude(JsonInclude.Include.NON_NULL)
data class ReceiveSubmitResponse(
    val fingerprint: String,
    val state: String,
    val timestamp: Long,
    val message: String? = null,
)
