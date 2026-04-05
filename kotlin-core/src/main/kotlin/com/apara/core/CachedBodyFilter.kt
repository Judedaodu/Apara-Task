package com.apara.core

import jakarta.servlet.FilterChain
import jakarta.servlet.ReadListener
import jakarta.servlet.ServletInputStream
import jakarta.servlet.http.HttpServletRequest
import jakarta.servlet.http.HttpServletRequestWrapper
import jakarta.servlet.http.HttpServletResponse
import org.springframework.core.Ordered
import org.springframework.core.annotation.Order
import org.springframework.stereotype.Component
import org.springframework.web.filter.OncePerRequestFilter
import java.io.ByteArrayInputStream

@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
class CachedBodyFilter : OncePerRequestFilter() {
    override fun doFilterInternal(
        request: HttpServletRequest,
        response: HttpServletResponse,
        filterChain: FilterChain,
    ) {
        if (request.requestURI != "/core/receive-submit" || request.method != "POST") {
            filterChain.doFilter(request, response)
            return
        }
        val body = request.inputStream.readAllBytes()
        val wrapped = object : HttpServletRequestWrapper(request) {
            override fun getInputStream(): ServletInputStream {
                val stream = ByteArrayInputStream(body)
                return object : ServletInputStream() {
                    override fun read(): Int = stream.read()
                    override fun read(b: ByteArray, off: Int, len: Int): Int = stream.read(b, off, len)
                    override fun isFinished(): Boolean = stream.available() == 0
                    override fun isReady(): Boolean = true
                    override fun setReadListener(readListener: ReadListener) {}
                }
            }
        }
        wrapped.setAttribute("cachedBody", body)
        filterChain.doFilter(wrapped, response)
    }
}
