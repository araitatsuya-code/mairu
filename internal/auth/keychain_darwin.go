//go:build darwin && cgo

package auth

/*
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

enum {
	mairuErrSecItemNotFound = errSecItemNotFound
};

static CFStringRef mairuCreateString(const char* value) {
	return CFStringCreateWithCString(kCFAllocatorDefault, value, kCFStringEncodingUTF8);
}

static OSStatus mairuSetGenericPassword(const char* service, const char* account, const void* secret, size_t secretLen) {
	CFStringRef serviceRef = mairuCreateString(service);
	CFStringRef accountRef = mairuCreateString(account);
	CFDataRef secretRef = CFDataCreate(kCFAllocatorDefault, (const UInt8*)secret, (CFIndex)secretLen);
	if (serviceRef == NULL || accountRef == NULL || secretRef == NULL) {
		if (serviceRef != NULL) {
			CFRelease(serviceRef);
		}
		if (accountRef != NULL) {
			CFRelease(accountRef);
		}
		if (secretRef != NULL) {
			CFRelease(secretRef);
		}
		return errSecAllocate;
	}

	const void* queryKeys[] = {
		kSecClass,
		kSecAttrService,
		kSecAttrAccount
	};
	const void* queryValues[] = {
		kSecClassGenericPassword,
		serviceRef,
		accountRef
	};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		queryKeys,
		queryValues,
		3,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL) {
		CFRelease(serviceRef);
		CFRelease(accountRef);
		CFRelease(secretRef);
		return errSecAllocate;
	}

	OSStatus status = SecItemCopyMatching(query, NULL);
	if (status == errSecSuccess) {
		const void* updateKeys[] = {
			kSecValueData
		};
		const void* updateValues[] = {
			secretRef
		};
		CFDictionaryRef updateAttrs = CFDictionaryCreate(
			kCFAllocatorDefault,
			updateKeys,
			updateValues,
			1,
			&kCFTypeDictionaryKeyCallBacks,
			&kCFTypeDictionaryValueCallBacks
		);
		if (updateAttrs == NULL) {
			CFRelease(query);
			CFRelease(serviceRef);
			CFRelease(accountRef);
			CFRelease(secretRef);
			return errSecAllocate;
		}
		status = SecItemUpdate(query, updateAttrs);
		CFRelease(updateAttrs);
	} else if (status == errSecItemNotFound) {
		const void* addKeys[] = {
			kSecClass,
			kSecAttrService,
			kSecAttrAccount,
			kSecValueData
		};
		const void* addValues[] = {
			kSecClassGenericPassword,
			serviceRef,
			accountRef,
			secretRef
		};
		CFDictionaryRef addQuery = CFDictionaryCreate(
			kCFAllocatorDefault,
			addKeys,
			addValues,
			4,
			&kCFTypeDictionaryKeyCallBacks,
			&kCFTypeDictionaryValueCallBacks
		);
		if (addQuery == NULL) {
			CFRelease(query);
			CFRelease(serviceRef);
			CFRelease(accountRef);
			CFRelease(secretRef);
			return errSecAllocate;
		}
		status = SecItemAdd(addQuery, NULL);
		CFRelease(addQuery);
	}

	CFRelease(query);
	CFRelease(serviceRef);
	CFRelease(accountRef);
	CFRelease(secretRef);
	return status;
}

static OSStatus mairuGetGenericPassword(const char* service, const char* account, void** dataOut, size_t* dataLenOut) {
	CFStringRef serviceRef = mairuCreateString(service);
	CFStringRef accountRef = mairuCreateString(account);
	if (serviceRef == NULL || accountRef == NULL) {
		if (serviceRef != NULL) {
			CFRelease(serviceRef);
		}
		if (accountRef != NULL) {
			CFRelease(accountRef);
		}
		return errSecAllocate;
	}

	const void* queryKeys[] = {
		kSecClass,
		kSecAttrService,
		kSecAttrAccount,
		kSecReturnData,
		kSecMatchLimit
	};
	const void* queryValues[] = {
		kSecClassGenericPassword,
		serviceRef,
		accountRef,
		kCFBooleanTrue,
		kSecMatchLimitOne
	};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		queryKeys,
		queryValues,
		5,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL) {
		CFRelease(serviceRef);
		CFRelease(accountRef);
		return errSecAllocate;
	}

	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	if (status == errSecSuccess) {
		CFDataRef dataRef = (CFDataRef)result;
		CFIndex dataLen = CFDataGetLength(dataRef);
		void* buffer = NULL;
		if (dataLen > 0) {
			buffer = malloc((size_t)dataLen);
			if (buffer == NULL) {
				status = errSecAllocate;
			} else {
				memcpy(buffer, CFDataGetBytePtr(dataRef), (size_t)dataLen);
			}
		}

		if (status == errSecSuccess) {
			*dataOut = buffer;
			*dataLenOut = (size_t)dataLen;
		}
	}

	if (result != NULL) {
		CFRelease(result);
	}
	CFRelease(query);
	CFRelease(serviceRef);
	CFRelease(accountRef);
	return status;
}

static OSStatus mairuDeleteGenericPassword(const char* service, const char* account) {
	CFStringRef serviceRef = mairuCreateString(service);
	CFStringRef accountRef = mairuCreateString(account);
	if (serviceRef == NULL || accountRef == NULL) {
		if (serviceRef != NULL) {
			CFRelease(serviceRef);
		}
		if (accountRef != NULL) {
			CFRelease(accountRef);
		}
		return errSecAllocate;
	}

	const void* queryKeys[] = {
		kSecClass,
		kSecAttrService,
		kSecAttrAccount
	};
	const void* queryValues[] = {
		kSecClassGenericPassword,
		serviceRef,
		accountRef
	};
	CFDictionaryRef query = CFDictionaryCreate(
		kCFAllocatorDefault,
		queryKeys,
		queryValues,
		3,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	if (query == NULL) {
		CFRelease(serviceRef);
		CFRelease(accountRef);
		return errSecAllocate;
	}

	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	CFRelease(serviceRef);
	CFRelease(accountRef);
	return status;
}

static char* mairuCopyErrorMessage(OSStatus status) {
	CFStringRef messageRef = SecCopyErrorMessageString(status, NULL);
	if (messageRef == NULL) {
		return NULL;
	}

	CFIndex length = CFStringGetLength(messageRef);
	CFIndex capacity = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
	char* buffer = (char*)calloc((size_t)capacity, 1);
	if (buffer == NULL) {
		CFRelease(messageRef);
		return NULL;
	}

	if (!CFStringGetCString(messageRef, buffer, capacity, kCFStringEncodingUTF8)) {
		free(buffer);
		buffer = NULL;
	}

	CFRelease(messageRef);
	return buffer;
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

type keychainStore struct {
	service string
}

func newKeychainStore(service string) SecretStore {
	return &keychainStore{service: service}
}

func (s *keychainStore) SetSecret(ctx context.Context, account string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	serviceValue := C.CString(s.service)
	accountValue := C.CString(account)
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))

	var secretPtr unsafe.Pointer
	if len(value) > 0 {
		secretPtr = unsafe.Pointer(&value[0])
	}

	status := C.mairuSetGenericPassword(
		serviceValue,
		accountValue,
		secretPtr,
		C.size_t(len(value)),
	)
	return mapKeychainError(status, "キーチェーンへの保存")
}

func (s *keychainStore) GetSecret(ctx context.Context, account string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	serviceValue := C.CString(s.service)
	accountValue := C.CString(account)
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))

	var dataPtr unsafe.Pointer
	var dataLen C.size_t

	status := C.mairuGetGenericPassword(
		serviceValue,
		accountValue,
		(*unsafe.Pointer)(unsafe.Pointer(&dataPtr)),
		&dataLen,
	)
	if status != C.OSStatus(0) {
		return nil, mapKeychainError(status, "キーチェーンからの読み出し")
	}
	if dataPtr == nil || dataLen == 0 {
		return []byte{}, nil
	}
	defer C.free(dataPtr)

	return C.GoBytes(dataPtr, C.int(dataLen)), nil
}

func (s *keychainStore) DeleteSecret(ctx context.Context, account string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	serviceValue := C.CString(s.service)
	accountValue := C.CString(account)
	defer C.free(unsafe.Pointer(serviceValue))
	defer C.free(unsafe.Pointer(accountValue))

	status := C.mairuDeleteGenericPassword(serviceValue, accountValue)
	return mapKeychainError(status, "キーチェーンからの削除")
}

func mapKeychainError(status C.OSStatus, action string) error {
	if status == C.OSStatus(0) {
		return nil
	}

	if status == C.OSStatus(C.mairuErrSecItemNotFound) {
		return ErrSecretNotFound
	}

	messagePtr := C.mairuCopyErrorMessage(status)
	if messagePtr != nil {
		defer C.free(unsafe.Pointer(messagePtr))
		message := C.GoString(messagePtr)
		if message != "" {
			return fmt.Errorf("%sに失敗しました: %w: %s", action, ErrSecretStoreUnavailable, message)
		}
	}

	return fmt.Errorf("%sに失敗しました: %w (status=%d)", action, ErrSecretStoreUnavailable, int(status))
}

var _ SecretStore = (*keychainStore)(nil)
