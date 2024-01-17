package proposals

import (
	"bytes"
	"fmt"

	"cosmossdk.io/log"
	cometabci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/skip-mev/slinky/abci/ve"
)

const (
	// NumInjectedTxs is the number of transactions that were injected into
	// the proposal but are not actual transactions. In this case, the oracle
	// info is injected into the proposal but should be ignored by the application.
	NumInjectedTxs = 1

	// OracleInfoIndex is the index of the oracle info in the proposal.
	OracleInfoIndex = 0
)

// The proposalhandler is responsible primarily for:
//  1. Filling a proposal with transactions.
//  2. Injecting vote extensions into the proposal (if vote extensions are enabled).
//  3. Verifying that the vote extensions injected are valid.
//
// To verify the validity of the vote extensions, the proposal handler will
// call the validateVoteExtensionsFn. This function is responsible for verifying
// that the vote extensions included in the proposal are valid and compose a
// supermajority of signatures and vote extensions for the current block.
type ProposalHandler struct {
	logger log.Logger

	// prepareProposalHandler fills a proposal with transactions.
	prepareProposalHandler sdk.PrepareProposalHandler

	// processProposalHandler processes transactions in a proposal.
	processProposalHandler sdk.ProcessProposalHandler

	// validateVoteExtensionsFn validates the vote extensions included in a proposal.
	validateVoteExtensionsFn ve.ValidateVoteExtensionsFn
}

// NewProposalHandler returns a new ProposalHandler.
func NewProposalHandler(
	logger log.Logger,
	prepareProposalHandler sdk.PrepareProposalHandler,
	processProposalHandler sdk.ProcessProposalHandler,
	validateVoteExtensionsFn ve.ValidateVoteExtensionsFn,
) *ProposalHandler {
	return &ProposalHandler{
		logger:                   logger,
		prepareProposalHandler:   prepareProposalHandler,
		processProposalHandler:   processProposalHandler,
		validateVoteExtensionsFn: validateVoteExtensionsFn,
	}
}

// PrepareProposalHandler returns a PrepareProposalHandler that will be called
// by base app when a new block proposal is requested. The PrepareProposalHandler
// will first fill the proposal with transactions. Then, if vote extensions are
// enabled, the handler will inject the extended commit info into the proposal.
func (h *ProposalHandler) PrepareProposalHandler() sdk.PrepareProposalHandler {
	return func(ctx sdk.Context, req *cometabci.RequestPrepareProposal) (*cometabci.ResponsePrepareProposal, error) {
		var (
			extInfoBz []byte
			err       error
		)

		if req == nil {
			h.logger.Error("prepare proposal received a nil request")
			return &cometabci.ResponsePrepareProposal{Txs: make([][]byte, 0)}, fmt.Errorf("received a nil request")
		}

		// If vote extensions are enabled, the current proposer must inject the extended commit
		// info into the proposal. This extended commit info contains the oracle data
		// for the current block.
		voteExtensionsEnabled := ve.VoteExtensionsEnabled(ctx)
		if voteExtensionsEnabled {
			h.logger.Info(
				"injecting oracle data into proposal",
				"height", req.Height,
				"vote_extensions_enabled", voteExtensionsEnabled,
			)

			extInfo := req.LocalLastCommit
			if err := h.ValidateExtendedCommitInfo(ctx, req.Height, extInfo); err != nil {
				h.logger.Error(
					"failed to validate vote extensions",
					"height", req.Height,
					"commit_info", extInfo,
					"err", err,
				)

				return &cometabci.ResponsePrepareProposal{Txs: make([][]byte, 0)}, err
			}

			// Inject the vote extensions into the proposal. These contain the oracle data
			// for the current block which will be committed to state in PreBlock.
			extInfoBz, err = extInfo.Marshal()
			if err != nil {
				h.logger.Error(
					"failed to extended commit info",
					"commit_info", extInfo,
					"err", err,
				)

				return &cometabci.ResponsePrepareProposal{Txs: make([][]byte, 0)}, err
			}

			req.Txs = append([][]byte{extInfoBz}, req.Txs...)
		}

		// Build the proposal.
		resp, err := h.prepareProposalHandler(ctx, req)
		if err != nil {
			h.logger.Error("failed to prepare proposal", "err", err)
			return &cometabci.ResponsePrepareProposal{Txs: make([][]byte, 0)}, err
		}

		// If the embedded prepare proposal handler threw out the injected data.
		// Re-inject it.
		if voteExtensionsEnabled && (len(resp.Txs) < 1 || !bytes.Equal(resp.Txs[0], extInfoBz)) {
			resp.Txs = append([][]byte{extInfoBz}, resp.Txs...)
		}

		h.logger.Info(
			"prepared proposal",
			"txs", len(resp.Txs),
			"vote_extensions_enabled", voteExtensionsEnabled,
		)

		return resp, nil
	}
}

// ProcessProposalHandler returns a ProcessProposalHandler that will be called
// by base app when a new block proposal needs to be verified. The ProcessProposalHandler
// will verify that the vote extensions included in the proposal are valid and compose
// a supermajority of signatures and vote extensions for the current block.
func (h *ProposalHandler) ProcessProposalHandler() sdk.ProcessProposalHandler {
	return func(ctx sdk.Context, req *cometabci.RequestProcessProposal) (*cometabci.ResponseProcessProposal, error) {
		voteExtensionsEnabled := ve.VoteExtensionsEnabled(ctx)

		h.logger.Info(
			"processing proposal",
			"height", req.Height,
			"num_txs", len(req.Txs),
			"vote_extensions_enabled", voteExtensionsEnabled,
		)

		if voteExtensionsEnabled {
			// Ensure that the commit info was correctly injected into the proposal.
			if len(req.Txs) < NumInjectedTxs {
				h.logger.Error("failed to process proposal: missing commit info", "num_txs", len(req.Txs))
				return &cometabci.ResponseProcessProposal{Status: cometabci.ResponseProcessProposal_REJECT},
					fmt.Errorf("missing commit info")
			}

			// Validate the vote extensions included in the proposal.
			extInfo := cometabci.ExtendedCommitInfo{}
			if err := extInfo.Unmarshal(req.Txs[OracleInfoIndex]); err != nil {
				h.logger.Error("failed to unmarshal commit info", "err", err)
				return &cometabci.ResponseProcessProposal{Status: cometabci.ResponseProcessProposal_REJECT},
					err
			}

			if err := h.ValidateExtendedCommitInfo(ctx, req.Height, extInfo); err != nil {
				h.logger.Error(
					"failed to validate vote extensions",
					"height", req.Height,
					"commit_info", extInfo,
					"err", err,
				)

				return &cometabci.ResponseProcessProposal{Status: cometabci.ResponseProcessProposal_REJECT},
					err
			}

			// Process the transactions in the proposal with the oracle data removed.
			req.Txs = req.Txs[NumInjectedTxs:]
		}

		return h.processProposalHandler(ctx, req)
	}
}
