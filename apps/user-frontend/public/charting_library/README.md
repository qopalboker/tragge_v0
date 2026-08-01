# TradingView Charting Library Setup

This directory should contain the TradingView Charting Library files.

## Installation

1. Obtain the TradingView Charting Library from TradingView:
   - Visit https://www.tradingview.com/HTML5-stock-forex-bitcoin-charting-library/
   - Request access to the library

2. Extract the library files to this directory. The structure should be:

```
charting_library/
├── charting_library.js
├── charting_library.d.ts
├── bundles/
├── datafeed/
└── ... (other files)
```

3. The chart will automatically load from `/charting_library/charting_library.js`

## Required Files

At minimum, ensure these files are present:
- `charting_library.js` - Main library script
- `bundles/` directory - Contains library bundles
- Static resources the library needs

## Notes

- The library is not included in the repository due to licensing
- Contact TradingView for licensing information
- The chart component will show an error message if the library is not found
