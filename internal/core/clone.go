package core

import "math/big"

func (finding Finding) Clone() Finding {
	finding.Evidence = cloneEvidence(finding.Evidence)
	return finding
}

func (result MonitorResult) Clone() MonitorResult {
	result.Metrics = cloneMetrics(result.Metrics)
	result.Findings = cloneFindings(result.Findings)
	return result
}

func (snapshot PositionSnapshot) Clone() PositionSnapshot {
	snapshot.ShareBalance = cloneBigInt(snapshot.ShareBalance)
	return snapshot
}

func (request FullExitRequest) Clone() FullExitRequest {
	request.Shares = cloneBigInt(request.Shares)
	return request
}

func (candidate TxCandidate) Clone() TxCandidate {
	candidate.Data = cloneBytes(candidate.Data)
	candidate.Value = cloneBigInt(candidate.Value)
	return candidate
}

func (simulation FullExitSimulation) Clone() FullExitSimulation {
	simulation.ExpectedAssetUnits = cloneBigInt(simulation.ExpectedAssetUnits)
	return simulation
}

func cloneBigInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	clone := make([]byte, len(value))
	copy(clone, value)
	return clone
}

func cloneMetrics(metrics []Metric) []Metric {
	if metrics == nil {
		return nil
	}
	clone := make([]Metric, len(metrics))
	copy(clone, metrics)
	return clone
}

func cloneFindings(findings []Finding) []Finding {
	if findings == nil {
		return nil
	}
	clone := make([]Finding, len(findings))
	for index, finding := range findings {
		clone[index] = finding.Clone()
	}
	return clone
}

func cloneEvidence(evidence map[string]string) map[string]string {
	if evidence == nil {
		return nil
	}
	clone := make(map[string]string, len(evidence))
	for key, value := range evidence {
		clone[key] = value
	}
	return clone
}
