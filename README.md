# Algorithmic Trading Orchestration Tool

## 📋 Project Overview

I have here the first draft of an algorithmic trading orchestration tool.

### Core Functionality
This application is configured to allow a single user to:
- Switch between different trading strategies
- Enable/disable base ERC20 tokens or any custom input ERC20 tokens that Coinbase has
- Any tokens that get enabled will effectively wind up triggering the manager to spawn a new trader and signaler for that token
  - The signaler will read price/candle data to generate buy/sell signals
  - The traders will be responsible for acting on those signals by submitting orders and adjusting its confirmed balance upon receiving order updates

### Architecture Overview
- **UI and backend Manager**: Singular instances
- **Per-token goroutines**: 2 goroutines for each enabled token (trader + signaler)
- **Additional goroutines**: For managing websockets to Coinbase and this application's own frontend
- **Data source**: Price and candle data from a websocket through the Coinbase Advanced Trade API
- **Strategy independence**: Each strategy is unique and independent of each other

### Graceful Shutdown System
Graceful closeout is important, so each goroutine inherits a context from their parent:
- When the parent is canceled (due to the profit/loss threshold being met or the user toggling trading activity off), the children will start to shut themselves off
- This generally happens instantly with everything except for the traders, which will attempt to closeout their positions before closing
- **Known issue**: Not all orders succeed, and the trader has been instructed to shutoff and will inevitably be shut off by the parent if it doesn't comply
- **Solution**: When the application starts a particular token, we pull the balance of that token from the exchange and initialize the trader/signaler with that

---

## 🏗️ Core Entities

### The Trader Entity
The Trader does 3 things:

1. **Signal ingestion**: Ingests buy/sell signals from the signaler, which buy/sell signals have a strength rating from 0-100% that is used to adjust the 'target position as a percentage of allocated funds'

2. **Trade submission**: Regularly submits trades if our actual token balance is out of tolerance

3. **Balance tracking**: Ingests order updates from the websocket that the Manager entity manages, uses those order updates to update the confirmed token balance

### The Manager Entity
The Manager does the following:

1. **Resource management**: Lives in the context of the API, so the Manager entity is what the API uses to spawn traders and signalers and the holder of the centralized data transfer and goroutine context resources

2. **Channel orchestration**: Manages all of the channels that need to be shared with different goroutines so they can communicate with each other as well as the frontend

3. **Shutdown coordination**: Responsible for orchestrating a timely, graceful and yet strict trader shutdown logic

### The Signaler Entity
The Signaler does the following:

1. **Data retrieval**: Accesses the Price Action Store to retrieve the most recent relevant candles for that signaler's assigned token

2. **Signal generation**: Processes that candle data into buy/sell signals from a series of financial indicator calculations and comparisons

3. **State synchronization**: Each strategy exposes a method for the Signal Engine to call to update the strategy about the state of the signals the trader has successfully received

#### Non-Blocking Design Philosophy
The reason for the above #3 bullet point is that all these goroutines should block each other as little as possible:
- Because they have to read from the same resources sometimes, which requires locks, we make sure not to sit at a channel trying to wait for a reader to accept our write (or vice versa) for too long
- This means some buy/sell signals will get dropped, which is fine because they get recalculated in a few seconds in that case

---

## 🚧 Current Work: Smart Contract Integration

Similar to how the user can select a particular strategy that will switch what classes get used for signal generation, I would like the user to also be able to toggle a single switch that flips the selected exchange back and forth between Coinbase's Advanced Trade API and Uniswap on the Ethereum blockchain.

### Smart-Contract/Blockchain Workflow Goals
Currently working on a smart-contract/blockchain workflow that accomplishes 4 things:

1. **MetaMask integration**: Enables me to connect my MetaMask to use it to interact with smart contracts

2. **Price data retrieval**: Cheaply and efficiently interacts with Etherscan or some other blockchain scanner for price and candle data, if possible
   - Because of the nature of the blockchain, I suppose I could see how that would be tricky or infeasible to emulate the existing candle stream that I get from Coinbase
   - I'd like to look into what is possible and what are the idiomatic ways other developers get such data regarding Uniswap prices

3. **USD to stablecoin conversion**: Pre-empting that the blockchain doesn't deal in USD but rather ERC20 tokens like USDC or USDT, etc, I need to be able to leverage my existing Coinbase connection to convert my USD into USDC when I switch my exchange toggle from Coinbase to Uniswap
   - Alternatively, I could have a trader manage a buy signal in two parts: one to Coinbase for the USDC/USDT and another to the smart contracts that will effect my trade through my MetaMask signature

4. **Trade execution**: Submits buy/sells to Uniswap through smart contract transactions
   - It's possible that it may make sense for me to deploy my own personal smart contracts that only my own wallet can interact with, that will manage my trades and other unique data management and utility operations
   - But I could also see the simplest strategy starting with just interacting directly with Uniswap smart contracts

---

## ✅ To-Do Lists

### Simple To-Dos
I also have a couple extra items I didn't fully fledge out yet that will need to be completed:

1. **Strategy initialization bug**: At the moment the strategies are not initialized with the initial token balance that the trader gets when it starts, which means we have to install handling to pass the old token balance from one strategy to another and also to give the strategy the token balances that the trader was intialized with upon startup

2. **UI performance metrics**: The manager does ingest order data and pass it off to the trader so it can know the state of its confirmed position, but the UI does not fully process this order data for the display of performance data metrics and graphs and trade history yet
   - At the moment, I suspect that will happen after I build out the Uniswap portion

3. **Profit/loss monitoring**: I also need to add a little goroutine that will constantly monitor the profit/loss that has accumulated throughout the day and if that threshold is reached it will automatically shut off all trading activity

---

## 🎯 Future Vision

When the above is finished, the first draft of the original vision of the project will be completed.

However, while I was working on this, I learned that I can also implement the following important features that when incorporated will enhance the power and flexibility that this application has:

### Planned Enhancements

1. **User-configurable strategies**: All strategies should be user configurable instead of only having hard-coded default values for their configuration/settings
   - That means a form can be exposed in the UI for the user to modulate each parameter mid-activity

2. **Universal candle filters**: All strategies should be able to use Heikin-Ashi or Renko candles without exception, because this is not necessarily a "strategy" as much as a noise reduction filter

3. **Risk management controls**: All strategies should have configurable take profit, stop loss, trailing stop
   - This one is already 90% true as I implemented it recently

4. **Deribit integration**: We should use Deribit instead of Coinbase
   - Because the Coinbase Advanced Trade API does not offer options, these trading strategies are stifled
   - Shorts are turned off completely on all strategies
   - Deribit does options and has an API that would meet this application's needs in full

5. **Configurable timeframes**: The length of the candles, i.e. the timescale that is analyzed/traded upon, should be user configurable down to 1-5 minute candles and up to 2-4 hour candles
   - This is not that hard to do right now, as we can build a goroutine to build candles from Coinbase price history requests upon token start and price streams as live data materializes
   - This will affect the rate limiting thresholds of the traders' order submissions and the signaler's signal calculations

6. **Position sizing controls**: The user should be able to configure 2 fractions
   - One is the % (of allocated funds) that an initial long/short signal will be
   - The second is the % that continued long trend confirmations will be
   - Reverse trends (i.e. a short signal when we are already long or vice versa) will always affect a 100% sell off of the current position 