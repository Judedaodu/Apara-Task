package com.apara.core

import org.springframework.boot.autoconfigure.SpringBootApplication
import org.springframework.boot.runApplication

@SpringBootApplication
class KotlinCoreApplication

fun main(args: Array<String>) {
    runApplication<KotlinCoreApplication>(*args)
}
