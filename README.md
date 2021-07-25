# BSC Web3 Syncer 

One day, I bought a very big hard drive and was planning to setup a BSC fullNode at home. My computer is behind serveral NAT devices, it is barely possible to connect to any active nodes. I am only using this fullNode for data analysis, so I think a delay of a few minutes is acceptable. So I built this simple pgrogram to  `synchronize a local BSC fullNode from Web3 RPC endpoint`.  



## Build 

simply clone and then:  

```
cd bsc-web3-syncer && go build -v 
```


## Usage 

```
Usage:
  bsc-web3-syncer [OPTIONS]

Application Options:
  -d, --datadir=       The data directory for the node database. A snapshot can be downloaded from
                       https://github.com/binance-chain/bsc-snapshots
  -e, --web3-endpoint= The PRC endpoint (ankr, getblock, etc.. or your own node somewhere else)
  -b, --batch=         The batch size for fetching blocks from RPC endpoint.
  -a, --archive        If this flag is set, trie writing cache and GC are both disabled

Help Options:
  -h, --help           Show this help message
```


## Example 


First, download the latest BSC snapshot from [here](https://github.com/binance-chain/bsc-snapshots). On July 24th of 2021, this zip file is about 300GB, it may take a little while to download it. After downloading the zip file, unzip it to somewhere (`/your/path/` for example).  

Next, go to [Ankr](https://app.ankr.com/api) and create a free BSC API. There will be two endpoints address for each created API. And we will use the `https` endpoint in the following example (`https://apis.ankr.com/xxxxxxxxxxxx/xxxxxxxxxxxxx/binance/full/main` for example).


Now we let's sync our local blockchain with Ankr endpoint:  

```
bsc-web3-syncer -d /your/path/server/data-seed \
                -s https://apis.ankr.com/xxxxxxxxxxxx/xxxxxxxxxxxxx/binance/full/main 
```


Trouble shooting, if you are using the free API from Ankr, you may run into such issue:  

```
ERROR[07-24|10:47:14.259] failed to fetch block                    error="429 Too Many Requests: request over flow one second" number=8,707,096
```

That means you need to pay a few bucks to remove such contraints. Or you can tweak the code by adding some sleeps in the `block fetching routine`.  (Maybe you can create multiple endpoints and build a pool of endpoints.)  


> BTW, you can also use the websocket endpoint(which starts with `wss`), it works better as a single TCP connection will be shared.  