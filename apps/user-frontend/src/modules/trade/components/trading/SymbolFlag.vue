<template>
  <div :class="containerClass">
    <div :class="['tp-flag-icon', flagClass(base)]"></div>
    <div v-if="quote" :class="['tp-flag-icon', flagClass(quote)]"></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  symbol: string
  base: string
  quote: string | null
}>()

const containerClass = computed(() => {
  return props.quote ? 'tp-flag-pair' : 'tp-flag-single'
})

function flagClass(currency: string): string {
  const curr = currency.toLowerCase()

  // Fiat currencies
  const fiatMap: Record<string, string> = {
    eur: 'tp-flag-eur',
    usd: 'tp-flag-usd',
    gbp: 'tp-flag-gbp',
    jpy: 'tp-flag-jpy',
    aud: 'tp-flag-aud',
    chf: 'tp-flag-chf',
    cad: 'tp-flag-cad',
    nzd: 'tp-flag-nzd',
    cny: 'tp-flag-cny',
    cnh: 'tp-flag-cny',
    sgd: 'tp-flag-sgd',
    hkd: 'tp-flag-hkd',
  }

  if (fiatMap[curr]) return fiatMap[curr]

  // Commodities
  const commodityMap: Record<string, string> = {
    xau: 'tp-flag-xau',
    xag: 'tp-flag-xag',
    wti: 'tp-flag-wti',
    oil: 'tp-flag-wti',
    gas: 'tp-flag-gas',
    natgas: 'tp-flag-gas',
  }

  if (commodityMap[curr]) return commodityMap[curr]

  // Indices
  const indexMap: Record<string, string> = {
    de: 'tp-flag-de',
    ger: 'tp-flag-de',
    ger40: 'tp-flag-de',
    us: 'tp-flag-us',
    us500: 'tp-flag-us',
    nas100: 'tp-flag-us',
    dj30: 'tp-flag-us',
    jp: 'tp-flag-jp',
    ni225: 'tp-flag-jp',
    uk: 'tp-flag-uk',
    uk100: 'tp-flag-uk',
  }

  if (indexMap[curr]) return indexMap[curr]

  // Crypto
  const cryptoMap: Record<string, string> = {
    btc: 'tp-flag-btc',
    eth: 'tp-flag-eth',
    usdt: 'tp-flag-usdt',
    sol: 'tp-flag-sol',
    bnb: 'tp-flag-bnb',
    xrp: 'tp-flag-xrp',
    ada: 'tp-flag-ada',
    doge: 'tp-flag-doge',
    dot: 'tp-flag-dot',
    link: 'tp-flag-link',
    avax: 'tp-flag-avax',
    matic: 'tp-flag-matic',
    pol: 'tp-flag-matic',
    atom: 'tp-flag-atom',
    ltc: 'tp-flag-ltc',
    uni: 'tp-flag-uni',
    near: 'tp-flag-near',
    apt: 'tp-flag-apt',
    sui: 'tp-flag-sui',
    arb: 'tp-flag-arb',
  }

  if (cryptoMap[curr]) return cryptoMap[curr]

  // Default - use USD style
  return 'tp-flag-usd'
}
</script>
