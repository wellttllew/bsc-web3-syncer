package main

import (
	"context"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jessevdk/go-flags"
)

// Sync a local binance smart chain node from a web3 endpoint (eg: ankr, getblock, etc)

func main() {

	var opts struct {
		DataDir     string `required:"true" short:"d" long:"datadir" description:"The data directory for the node database. A snapshot can be downloaded from https://github.com/binance-chain/bsc-snapshots"`
		RPCEndpoint string `required:"true" short:"e" long:"web3-endpoint" description:"The PRC endpoint (ankr, getblock, etc..)"`
		BatchSize   uint64 `short:"b" long:"batch" description:"The batch size for fetching blocks from RPC endpoint."`
		ArchiveNode bool   `short:"a" long:"archive" description:"If this flag is set, trie writing cache and GC are both disabled"`
	}

	logger := log.New()

	_, err := flags.Parse(&opts)
	if err != nil {
		logger.Crit("failed to parse commandline arguments", "error", err)
	}

	if opts.BatchSize == 0 {
		opts.BatchSize = 1 // default batch size
	}

	nodeCfg := node.Config{
		Name:                "geth",
		DataDir:             filepath.Clean(opts.DataDir),
		HTTPModules:         []string{"net", "web3"},
		HTTPVirtualHosts:    []string{"localhost"},
		WSModules:           []string{"net", "web3"},
		GraphQLVirtualHosts: []string{"localhost"},
		P2P: p2p.Config{
			MaxPeers:    50,
			NAT:         nat.Any(),
			NoDiscovery: true, // do not discover new peers
		},
		Logger: logger,
	}

	stack, err := node.New(&nodeCfg)
	MustBeNil(err)

	ethCfg := ethconfig.Defaults
	ethCfg.SnapshotCache = 0 // no snapshot cache
	ethCfg.TxLookupLimit = 0 // keep all indexes
	if opts.ArchiveNode {
		ethCfg.NoPruning = true
		ethCfg.TrieDirtyCache = 0
	}

	// TODO: You may want to tune other cache related configs

	// Although we are creating a new ethereum object here,
	// We are not going to Start it.
	ethObj, err := eth.New(stack, &ethCfg)
	MustBeNil(err)

	ctx, cancel := NewSingalContext()

	blocksC := make(chan types.Blocks, 10)
	wg := &sync.WaitGroup{}

	// Routine for fetching blocks from web3 Endpoints
	wg.Add(1)
	go func() {
		defer logger.Info("Block Fetching Routine Exited")
		defer wg.Done()
		defer cancel()

		// web3 client
		ethCli := NewEthClient(opts.RPCEndpoint)
		lastFetchedBlock := ethObj.BlockChain().CurrentBlock().NumberU64()

		for {

			latestBlock, err := ethCli.BlockNumber(ctx)
			if err != nil {
				logger.Warn("failed to get block number", "error", err)
				return
			}

			if latestBlock <= lastFetchedBlock {
				logger.Info("Waiting 3 seconds for new block to come",
					"localBlockNumber", lastFetchedBlock, "remoteBlockNumber", latestBlock)
				timer := time.NewTimer(time.Second * 3)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}

			batchSync := &sync.WaitGroup{}

			// fetch blocks in batch
			for b := lastFetchedBlock + 1; b <= latestBlock; {
				currentBatchSize := Min(latestBlock+1-b, opts.BatchSize)
				blocks := make([]*types.Block, currentBatchSize)

				for i := uint64(0); i < currentBatchSize; i++ {
					batchSync.Add(1)
					go func(index uint64) {
						defer batchSync.Done()

						logger.Info("fetching block from rpc endpoint", "number", b+index)
						block, err := ethCli.BlockByNumber(ctx, big.NewInt(int64(b+index)))
						if err != nil {
							logger.Error("failed to fetch block", "error", err, "number", b+i)
							return
						}
						blocks[index] = block
					}(i)
				}
				batchSync.Wait()

				select {
				case <-ctx.Done():
					return
				case blocksC <- types.Blocks(blocks):
				}

				b += currentBatchSize
				lastFetchedBlock += currentBatchSize
			}

		}

	}()

	// Routine for inserting blocks into chain
	wg.Add(1)
	go func() {
		defer logger.Info("block insertion routine exited")
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case blocs := <-blocksC:
				_, err = ethObj.BlockChain().InsertChain(blocs)
				if err != nil {
					logger.Error("failed to insert blocks into chain", "error", err)
					return
				}
			}
		}

	}()

	// Sync routines above and exit
	wg.Wait()

	// Flush caches to disk and do some cleanups
	// We cannot call ethObj.Stop(), because we didn't actually start it.
	func() {
		ethObj.TxPool().Stop()
		ethObj.BlockChain().Stop() // flush caches to disk
		ethObj.Miner().Stop()
		ethObj.Engine().Close()
		rawdb.PopUncleanShutdownMarker(ethObj.ChainDb())
		ethObj.ChainDb().Close()
		ethObj.EventMux().Stop()
	}()

}

func NewEthClient(endpoint string) *ethclient.Client {

	// rpc client
	rpcClient, err := rpc.DialContext(context.Background(), endpoint)
	MustBeNil(err)

	// eth client
	ec := ethclient.NewClient(rpcClient)

	return ec
}

func MustBeNil(err error) {
	if err != nil {
		panic(err)
	}
}

// Create a new context
// The returned ctx.Done() will be closed if SIGINT is received.
func NewSingalContext() (ctx context.Context, cancel context.CancelFunc) {

	ctx, cancel = context.WithCancel(context.Background())

	go func() {
		// Handle linux signals
		exitC := make(chan os.Signal, 1)
		signal.Notify(exitC, os.Interrupt, syscall.SIGTERM)

		<-exitC
		cancel()
	}()

	return ctx, cancel

}

func Min(a, b uint64) uint64 {
	if a > b {
		return b
	}

	return a
}
