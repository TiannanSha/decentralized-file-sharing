package impl

import (
	"crypto"
	"encoding/hex"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/peer"
	"io"
)

// Upload create and store in the BlobStore the chunks and metafile
func (n* node) Upload(data io.Reader) (metahash string, err error) {

	isFirstChunk := true;
	var MetafileValue string
	var MetafileKeyBytes []byte;
	for {
		buf := make([]byte, n.conf.ChunkSize)
		numBytes, err := data.Read(buf)
		if err == io.EOF {
			// no more bytes to read
			break
		}
		//fmt.Printf("n = %v err = %v buf = %v\n", n, err, buf)
		//fmt.Printf("b[:n] = %q\n", buf[:n])
		chunk := newChunk(buf, numBytes)
		n.conf.Storage.GetDataBlobStore().Set(chunk.hashHexStr, chunk.dataBuf[0:chunk.dataLen])

		// gradually build metafile vale. for first chunk, no separator is needed
		if (isFirstChunk) {
			MetafileValue += chunk.hashHexStr;
			isFirstChunk = false
		} else {
			MetafileValue += (peer.MetafileSep + chunk.hashHexStr)
		}

		// gradually build metafile key bytes
		MetafileKeyBytes = append(MetafileKeyBytes, chunk.hash...)
	}

	// put MetafileValue and its hash in the key value store
	test := hex.EncodeToString(MetafileKeyBytes)
	log.Info().Msgf(test)
	MetafileKey := hashThenEncodeToHexStr(MetafileKeyBytes, len(MetafileKeyBytes))
	n.conf.Storage.GetDataBlobStore().Set(MetafileKey, []byte(MetafileValue))
	return MetafileKey, nil
}

type Chunk struct {
	dataBuf []byte
	hash    []byte
	dataLen    int
	hashHexStr string
}

func newChunk(data []byte, dataLen int) Chunk {
	hash := hashDataToBytes(data, dataLen)
	return Chunk{dataBuf: data, hash: hash, hashHexStr: hex.EncodeToString(hash), dataLen: dataLen}
}

// first do sha256 hash, then do hex encoding
func hashThenEncodeToHexStr(data []byte, dataLen int) string {
	hash := hashDataToBytes(data[0:dataLen], dataLen)
	hashHex := hex.EncodeToString(hash)
	return hashHex
}

func hashDataToBytes(data []byte, dataLen int) []byte {
	h := crypto.SHA256.New()
	h.Write(data[0:dataLen])
	return h.Sum(nil)
}


